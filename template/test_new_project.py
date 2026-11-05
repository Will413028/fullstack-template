"""Tests for new_project.py. Run: python3 -m unittest discover -s template

CI's generate job runs the whole derived project's checks; these cover the generator's own rules on a small git
repository built in a temporary directory, so they run anywhere in a second or two.
"""

import importlib.util
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

SCRIPT = pathlib.Path(__file__).resolve().parent / "new_project.py"
_spec = importlib.util.spec_from_file_location("new_project", SCRIPT)
new_project = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(new_project)

NAME = new_project.OLD_NAME
DB = new_project.OLD_DB
REPO = new_project.OLD_REPO
GIT_ENV = {
    "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.test",
    "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.test",
    "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1",
}


def git(cwd: pathlib.Path, *args: str) -> str:
    # never the GIT_DIR a hook running these tests exports: it would point the commands at the real repository
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True,
                          env={**{k: v for k, v in os.environ.items() if not k.startswith("GIT_")}, **GIT_ENV}).stdout


def make_template(root: pathlib.Path, files: dict[str, str]) -> pathlib.Path:
    src = root / "src"
    src.mkdir()
    for rel, text in files.items():
        path = src / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
    git(src, "init", "-q", "-b", "main")
    git(src, "add", "-A")
    git(src, "commit", "-q", "-m", "template")
    return src


BASE_FILES = {
    "package.json": f'{{"name": "{NAME}"}}\n',
    "backend/go.mod": f"module github.com/{REPO}/backend\n",
    "SECURITY.md": f"report at https://github.com/{REPO}/security/advisories/new\n",
    "docker-compose.yml": f"POSTGRES_DB: {DB}\nAPP_USER: {DB}_app\n",
    "README.md": f"# {NAME}\ntemplate readme: make new\n",
    "template/derived/README.md": f"# {NAME}\nderived readme\n",
    "template/new_project.py": "# the generator\n",
    "docs/plans/plan.md": f"{NAME} plan\n",
    ".github/workflows/ci.yml": "jobs:\n  app: {}\n  # >>> template-only\n  generate: {}\n  # <<< template-only\n",
}


class GenerateTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.tmp.name)
        self.env = mock.patch.dict(os.environ, GIT_ENV)
        self.env.start()
        # 027, not the usual 022: the data filter alone already drops group and other write bits
        self.old_umask = os.umask(0o027)
        # regenerating and building the backend needs Go and buf: CI's generate job runs the real one
        self.finish = mock.patch.object(new_project, "finish_backend")
        self.finish_backend = self.finish.start()

    def tearDown(self):
        self.finish.stop()
        self.env.stop()
        os.umask(self.old_umask)
        self.tmp.cleanup()

    def case(self, label: str) -> pathlib.Path:
        """A fresh directory of its own for one subTest case."""
        root = self.root / label
        root.mkdir()
        return root

    def generate(self, files=None, name="my-app", repo=None, dest=None):
        src = make_template(self.root, BASE_FILES if files is None else files)
        dest = dest or self.root / "out"
        return src, dest, new_project.generate(src, dest, name, repo or f"acme/{name}")

    def read(self, dest: pathlib.Path, rel: str) -> str:
        return (dest / rel).read_text(encoding="utf-8")

    def test_a_git_hooks_environment_does_not_reach_another_repository(self):
        # a git hook exports GIT_DIR (and in a linked worktree more); generate() and its git calls must ignore it
        outer = self.root / "outer"
        outer.mkdir()
        git(outer, "init", "-q", "-b", "dev")
        git(outer, "commit", "-q", "--allow-empty", "-m", "outer")
        hook_env = {"GIT_DIR": str(outer / ".git"), "GIT_INDEX_FILE": str(outer / ".git/index")}
        src = make_template(self.root, BASE_FILES)
        with mock.patch.dict(os.environ, hook_env):
            new_project.generate(src, self.root / "out", "my-app", "acme/my-app")
        self.assertEqual(git(outer, "config", "core.bare").strip(), "false")
        self.assertEqual(git(outer, "rev-list", "--all", "--count").strip(), "1")
        self.assertEqual(git(self.root / "out", "rev-list", "--all", "--count").strip(), "1")

    def test_replaces_each_token_in_contents_and_names(self):
        _, dest, _ = self.generate()
        self.assertEqual(self.read(dest, "package.json"), '{"name": "my-app"}\n')
        self.assertIn("github.com/acme/my-app/security", self.read(dest, "SECURITY.md"))
        self.assertEqual(self.read(dest, "docker-compose.yml"), "POSTGRES_DB: my_app\nAPP_USER: my_app_app\n")
        # the Go module path follows the repository
        self.assertEqual(self.read(dest, "backend/go.mod"), "module github.com/acme/my-app/backend\n")

    def test_the_backend_is_finished_on_the_replaced_tree_before_the_commit(self):
        seen = {}
        self.finish_backend.side_effect = lambda out: seen.update(
            go_mod=(out / "backend/go.mod").read_text(encoding="utf-8"), git=(out / ".git").exists())
        self.generate()
        self.assertEqual(seen, {"go_mod": "module github.com/acme/my-app/backend\n", "git": False})

    def test_a_backend_that_does_not_build_writes_nothing(self):
        self.finish_backend.side_effect = new_project.GenerateError("go build ./... failed")
        self.assert_refused_and_untouched("go build")

    def test_a_path_naming_the_template_writes_nothing(self):
        # file names are not rewritten: a name that would need it is refused instead of renamed
        self.assert_refused_and_untouched("the path holds", files={**BASE_FILES, f"{NAME}.txt": "named after the template\n"})

    def test_a_repository_that_names_the_project_is_replaced_once(self):
        # sequential str.replace rewrote acme/<name>-fork a second time; one pass never rescans its output
        _, dest, _ = self.generate(repo=f"acme/{NAME}-fork")
        self.assertIn(f"github.com/acme/{NAME}-fork/security", self.read(dest, "SECURITY.md"))

    def test_template_only_files_and_blocks_are_left_out(self):
        _, dest, _ = self.generate()
        self.assertFalse((dest / "template").exists())
        self.assertFalse((dest / "docs/plans").exists())
        self.assertEqual(self.read(dest, "README.md"), "# my-app\nderived readme\n")
        self.assertEqual(self.read(dest, ".github/workflows/ci.yml"), "jobs:\n  app: {}\n")

    def test_starts_a_new_repository_with_one_commit_and_no_remote(self):
        src, dest, _ = self.generate()
        self.assertEqual(git(dest, "rev-list", "--count", "HEAD").strip(), "1")
        # one branch; the branch model is set up on the hosting side after the first push
        self.assertEqual(git(dest, "branch", "--format=%(refname:short)").split(), ["main"])
        self.assertEqual(oct(dest.stat().st_mode & 0o777), oct(0o750))
        self.assertEqual(oct((dest / "package.json").stat().st_mode & 0o777), oct(0o640))
        self.assertEqual(git(dest, "remote").strip(), "")
        self.assertEqual(git(dest, "status", "--porcelain"), "")
        sha = git(src, "rev-parse", "HEAD").strip()
        self.assertIn(f"Large-Change: initial commit generated from {NAME}@{sha}", git(dest, "log", "-1", "--format=%B"))

    def test_keeps_the_executable_bit(self):
        files = {**BASE_FILES, "run.sh": "#!/bin/sh\n"}
        src = make_template(self.root, files)
        (src / "run.sh").chmod(0o755)
        git(src, "add", "run.sh")
        git(src, "commit", "-q", "-m", "exec")
        dest = self.root / "out"
        new_project.generate(src, dest, "my-app", "acme/my-app")
        self.assertTrue(os.access(dest / "run.sh", os.X_OK))
        self.assertFalse(os.access(dest / "package.json", os.X_OK))

    def assert_refused_and_untouched(self, error, files=None, name="my-app", repo=None, dest_setup=None, root=None):
        """generate() fails with a message matching error (the check meant, not some other one) and writes nothing."""
        root = root or self.root
        dest = root / "out"
        if dest_setup:
            dest_setup(dest)
        before = sorted(p.name for p in root.iterdir())
        src = make_template(root, BASE_FILES if files is None else files)
        with self.assertRaisesRegex(new_project.GenerateError, error):
            new_project.generate(src, dest, name, repo or f"acme/{name}")
        after = sorted(p.name for p in root.iterdir() if p.name != "src")
        self.assertEqual(after, before, "a refused run left files next to the template")
        if dest.exists():
            self.assertEqual(sorted(p.name for p in dest.iterdir()), ["keep"] if dest_setup else [])

    def test_output_still_naming_the_template_writes_nothing(self):
        # a capitalised form the table does not cover: the check runs before any file is written
        self.assert_refused_and_untouched("still contains 'TEMPLATE-GO-NEXTJS'", files={**BASE_FILES, "notes.md": "TEMPLATE-GO-NEXTJS\n"})

    def test_output_referring_to_the_generator_writes_nothing(self):
        self.assert_refused_and_untouched("still contains 'new_project.py'",
                                          files={**BASE_FILES, "notes.md": "run python3 new_project.py\n"})

    def test_unbalanced_template_only_block_writes_nothing(self):
        self.assert_refused_and_untouched("never closed", files={**BASE_FILES, "x.yml": "a\n# >>> template-only\nb\n"})

    def test_refuses_a_destination_that_is_not_empty(self):
        self.assert_refused_and_untouched("not an empty directory", dest_setup=lambda d: (d.mkdir(), (d / "keep").write_text("x")))

    def test_an_empty_destination_directory_is_used(self):
        dest = self.root / "out"
        dest.mkdir()
        new_project.generate(make_template(self.root, BASE_FILES), dest, "my-app", "acme/my-app")
        self.assertTrue((dest / ".git").is_dir())

    def test_refuses_a_dirty_template(self):
        src = make_template(self.root, BASE_FILES)
        (src / "package.json").write_text("changed\n", encoding="utf-8")
        with self.assertRaisesRegex(new_project.GenerateError, "uncommitted"):
            new_project.generate(src, self.root / "out", "my-app", "acme/my-app")
        self.assertFalse((self.root / "out").exists())

    def test_a_git_failure_is_an_error_not_an_empty_template(self):
        notrepo = self.root / "notrepo"
        notrepo.mkdir()
        with self.assertRaisesRegex(new_project.GenerateError, r"git .* failed"):
            new_project.generate(notrepo, self.root / "out", "my-app", "acme/my-app")
        self.assertFalse((self.root / "out").exists())

    def test_a_failure_after_writing_leaves_nothing(self):
        # the files are written and git init ran when the commit fails (a hook, a missing identity)
        src = make_template(self.root, BASE_FILES)
        real = new_project.run_git

        def failing_commit(cwd, *args, **kwargs):
            if args[0] == "commit":
                raise new_project.GenerateError("commit failed")
            return real(cwd, *args, **kwargs)

        with mock.patch.object(new_project, "run_git", failing_commit), self.assertRaises(new_project.GenerateError):
            new_project.generate(src, self.root / "out", "my-app", "acme/my-app")
        self.assertEqual(sorted(p.name for p in self.root.iterdir()), ["src"])

    def test_output_referring_to_an_excluded_directory_writes_nothing(self):
        # (file, text, the reference reported); a relative path is resolved against the file that holds it
        cases = (("notes.md", "see `docs/plans/`\n", "docs/plans"),
                 ("notes.md", "see template/README.md\n", "template"),
                 ("docs/notes.md", "[plan](../docs/plans/x.md)\n", "../docs/plans"),
                 ("notes.md", "run ./template/x\n", "./template"),
                 ("notes.md", "run make new NAME=x\n", "make new NAME="),
                 # a relative link that names only the last part of an excluded directory
                 ("docs/privacy/notes.md", "[plan](../plans/x.md)\n", "../plans/x.md"),
                 ("backend/sub/notes.md", "see ../../template/x\n", "../../template/x"))
        for number, (rel, text, ref) in enumerate(cases):
            with self.subTest(rel=rel, text=text):
                self.assert_refused_and_untouched(f"{rel}: .*{ref}", files={**BASE_FILES, rel: text},
                                                  root=self.case(str(number)))

    def test_a_binary_file_referring_to_an_excluded_directory_writes_nothing(self):
        src = make_template(self.root, BASE_FILES)
        (src / "logo.bin").write_bytes(b"\xff\xfe see docs/plans/x\n")
        git(src, "add", "logo.bin")
        git(src, "commit", "-q", "-m", "binary")
        with self.assertRaisesRegex(new_project.GenerateError, "logo.bin: refers to the excluded 'docs/plans'"):
            new_project.generate(src, self.root / "out", "my-app", "acme/my-app")
        self.assertFalse((self.root / "out").exists())

    def test_ordinary_text_and_unrelated_template_directories_are_allowed(self):
        files = {**BASE_FILES,
                 "src/mail/x.ts": 'import "./template/body";\n',  # src/mail/template/, which the output keeps
                 "src/mail/template/body.ts": "export {};\n",
                 "src/mail/y.ts": 'import "./emails/template/a"; import "@/template/b"; // src/docs/plans/c\n',
                 "notes.md": "make new tables; the new_project table; ../template/ from the root leaves the tree\n"}
        _, dest, _ = self.generate(files=files)
        self.assertTrue((dest / "src/mail/template/body.ts").exists())

    def test_a_symlink_writes_nothing(self):
        # leaving the tree is stopped by the data filter while unpacking, a link inside it by the tree check
        cases = (("/etc/hosts", "'link' is a link to an absolute path"), ("README.md", "link: a symbolic link"))
        for number, (target, error) in enumerate(cases):
            with self.subTest(target=target):
                root = self.case(str(number))
                src = make_template(root, BASE_FILES)
                (src / "link").symlink_to(target)
                git(src, "add", "link")
                git(src, "commit", "-q", "-m", "link")
                with self.assertRaisesRegex(new_project.GenerateError, error):
                    new_project.generate(src, root / "out", "my-app", "acme/my-app")
                self.assertEqual(sorted(p.name for p in root.iterdir()), ["src"])

    def test_export_attributes_write_nothing(self):
        self.assert_refused_and_untouched(r"SECURITY\.md \(export-ignore\)",
                                          files={**BASE_FILES, ".gitattributes": "SECURITY.md export-ignore\n"})

    def test_export_attributes_outside_the_tree_write_nothing(self):
        # .git/info/attributes and core.attributesFile reach git archive too
        for number, where in enumerate(("info", "config")):
            with self.subTest(where=where):
                root = self.case(str(number))
                src = make_template(root, BASE_FILES)
                rules = root / "attributes"
                rules.write_text("README.md export-subst\n", encoding="utf-8")
                if where == "info":
                    (src / ".git/info").mkdir(exist_ok=True)
                    rules.replace(src / ".git/info/attributes")
                else:
                    git(src, "config", "core.attributesFile", str(rules))
                with self.assertRaisesRegex(new_project.GenerateError, r"README\.md \(export-subst\)"):
                    new_project.generate(src, root / "out", "my-app", "acme/my-app")
                self.assertFalse((root / "out").exists())

    def test_export_attributes_on_a_derived_file_write_nothing(self):
        self.assert_refused_and_untouched(r"template/derived/README\.md \(export-ignore\)",
                                          files={**BASE_FILES, "template/.gitattributes": "derived/** export-ignore\n"})

    def test_unset_and_commented_export_attributes_are_allowed(self):
        attributes = "# SECURITY.md export-ignore\nSECURITY.md -export-ignore\n*.bin export-ignore\n"
        # template/ outside derived/ is dropped after the export, so its attributes cannot change the output
        _, dest, _ = self.generate(files={**BASE_FILES, ".gitattributes": attributes,
                                          "template/.gitattributes": "new_project.py export-ignore\n"})
        self.assertTrue((dest / "SECURITY.md").exists())

    def test_a_derived_file_keeps_its_mode_and_may_add_a_directory(self):
        files = {**BASE_FILES, "template/derived/bin/run.sh": "#!/bin/sh\n"}
        src = make_template(self.root, files)
        (src / "template/derived/bin/run.sh").chmod(0o755)
        git(src, "add", "-A")
        git(src, "commit", "-q", "-m", "exec")
        dest = self.root / "out"
        new_project.generate(src, dest, "my-app", "acme/my-app")
        self.assertTrue(os.access(dest / "bin/run.sh", os.X_OK))

    def test_a_failed_move_gives_back_the_empty_destination(self):
        dest = self.root / "out"
        dest.mkdir()
        src = make_template(self.root, BASE_FILES)
        # an OSError (EBUSY on a mount point, a permission) is reported as an error, not a traceback
        with mock.patch.object(new_project.os, "rename", side_effect=OSError("busy")), \
                self.assertRaisesRegex(new_project.GenerateError, "busy"):
            new_project.generate(src, dest, "my-app", "acme/my-app")
        self.assertEqual(sorted(p.name for p in self.root.iterdir()), ["out", "src"])
        self.assertEqual(list(dest.iterdir()), [])

    def test_an_old_python_is_an_error_message(self):
        # tarfile's extraction filters came with Python 3.12 (and late 3.8-3.11 patch releases)
        with mock.patch.object(new_project, "tarfile", object()), \
                self.assertRaisesRegex(new_project.GenerateError, r"3\.12\+, 3\.11\.4\+"):
            new_project.generate(make_template(self.root, BASE_FILES), self.root / "out", "my-app", "acme/my-app")
        self.assertEqual(sorted(p.name for p in self.root.iterdir()), ["src"])

    def test_the_longest_name_still_fits_postgres(self):
        new_project.check_arguments("a" * 57, "acme/x")  # a57_owner is 63 bytes, PostgreSQL's limit

    def test_refuses_a_repository_that_is_not_a_path_segment(self):
        # it becomes the Go module path github.com/<repo>/backend: refused before anything is exported
        for repo in ("acme/..", "acme/.", "acme/.git", "acme/.hidden"):
            with self.subTest(repo=repo), self.assertRaisesRegex(new_project.GenerateError, "invalid repository"):
                new_project.check_arguments("my-app", repo)

    def test_refuses_bad_names_and_repositories(self):
        for name, repo in (("My-App", "acme/x"), ("my_app", "acme/x"), ("1app", "acme/x"), (NAME, "acme/x"),
                           ("a" * 58, "acme/x"), ("my-app", "acme"), ("my-app", "acme/x/y"), ("my-app", "acme/x y")):
            with self.subTest(name=name, repo=repo):
                with self.assertRaisesRegex(new_project.GenerateError, "invalid|longer|other than"):
                    new_project.check_arguments(name, repo)


class TemplateRepositoryTest(unittest.TestCase):
    """Rules about this repository's own files that the generator relies on."""

    def test_the_real_template_generates_cleanly(self):
        # writes the output of this checkout's HEAD to a temporary directory: every check except the clean-tree one
        top = SCRIPT.parent.parent
        with tempfile.TemporaryDirectory() as tmp:
            out = pathlib.Path(tmp) / "project"
            new_project.materialize(top, out, "my-app", "acme/my-app")
            self.assertTrue((out / "backend/go.mod").is_file())
            self.assertFalse((out / "template").exists())


if __name__ == "__main__":
    sys.exit(unittest.main())
