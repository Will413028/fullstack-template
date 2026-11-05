#!/usr/bin/env python3
"""Generate a new project from this template into a new directory.

Usage: python3 template/new_project.py <name> <dest> [--repo <owner/name>]   (make new NAME=... DEST=... [REPO=...])

The template stays a project that runs as it is. Its HEAD commit (never the working tree, which must be clean) is
exported with `git archive`, leaving out the paths in EXCLUDED, and unpacked with tarfile's "data" filter into a
temporary directory next to <dest>, the way create-next-app unpacks an example. There, files under template/derived/
replace their counterparts, the lines between a `>>> template-only` and a `<<< template-only` marker are dropped, and
every template name is replaced in one pass over file contents (file names are not rewritten: a path holding a token
is refused). The tree is then checked: nothing may still name the template, refer to an excluded path or hold a
marker. The Go module path (github.com/<repo>/backend) changes with the repository, and the generated Connect code
carries it inside each raw descriptor, length-prefixed, where a text replacement breaks it: the backend is regenerated
with the template's pinned buf, gofmt'ed and built. Only a clean tree that builds is committed (`git init -b main`, one commit, no remote) and moved onto <dest>, so a failure
leaves nothing behind. The branch model and branch protection are set up on the hosting side after the first push.
The new project has no link to the template: later template fixes do not reach it.
"""

from __future__ import annotations  # the version check in generate() must run on an old Python, not a SyntaxError

import argparse
import functools
import io
import os
import pathlib
import posixpath
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile

OLD_NAME = "template-go-nextjs"
OLD_DB = OLD_NAME.replace("-", "_")
OLD_REPO = f"Will413028/{OLD_NAME}"
DEFAULT_OWNER = "Will413028"
# Paths (prefixes) the export leaves out: the generator, its tests and the derived files' sources; the plans.
EXCLUDED = ("template/", "docs/plans/")
# template/derived/<path> replaces <path> in the output.
DERIVED = "template/derived/"
BEGIN, END = ">>> template-only", "<<< template-only"
# a file every template has at HEAD: a missing one means top is not the template
LANDMARK = "backend/go.mod"
# the generator's file and its make target, in the exact forms a command or a link uses
GENERATOR = pathlib.Path(__file__).name  # new_project.py
MAKE_TARGET = "make new NAME="
# PostgreSQL truncates identifiers at 63 bytes; the longest one made from the name is the role <name>_owner.
MAX_NAME = 63 - len("_owner")
NAME_PATTERN = re.compile(r"[a-z](?:[a-z0-9-]*[a-z0-9])?")
# the name part starts with a letter, digit or underscore: not ., .. or .git (it becomes the Go module path)
REPO_PATTERN = re.compile(r"[A-Za-z0-9-]+/[A-Za-z0-9_-][A-Za-z0-9._-]*")
# What the output must not hold, derived from the tables above: a template name in any case; the generator and its make
# target; a marker.
FORBIDDEN = re.compile(
    "|".join(re.escape(s) for s in (OLD_REPO, OLD_DB, OLD_NAME, GENERATOR, MAKE_TARGET, BEGIN, END)),
    re.IGNORECASE,
)
# A path ending in an excluded directory's name, at a path boundary, with any leading ./ and ../ (checked by resolving it
# against the referring file: src/mail/x.ts's ./template/ is src/mail/template/, which the output keeps).
PATH_REF = re.compile(
    r"(?<![\w./@-])((?:\.\.?/)*)("
    # template/ needs its slash (the word alone is ordinary text); docs/plans is a path either way
    + "|".join(re.escape(prefix) if "/" not in prefix.rstrip("/") else re.escape(prefix.rstrip("/")) + r"(?![\w.-])"
               for prefix in EXCLUDED)
    + ")"
)
# Any relative path (./ or ../ followed by at least one segment): resolved against the referring file, it may land in an
# excluded directory without naming it (docs/x.md's ../plans/y.md is docs/plans/y.md).
RELATIVE_REF = re.compile(r"(?<![\w./@-])(?:\.\.?/)+[\w.-]+(?:/[\w.-]+)*")
# What git archive leaves out; template/ is exported for its derived files and dropped after they are moved.
EXPORTED_EXCLUDED = tuple(prefix for prefix in EXCLUDED if not DERIVED.startswith(prefix))
# git archive drops export-ignore paths and rewrites export-subst files; the output must be the committed files
EXPORT_ATTRIBUTES = ("export-ignore", "export-subst")


class GenerateError(Exception):
    pass


@functools.cache
def _local_git_vars() -> frozenset[str]:
    """The variables that point git at a repository (`git rev-parse --local-env-vars`), asked once per run."""
    try:
        names = subprocess.run(["git", "rev-parse", "--local-env-vars"], capture_output=True, text=True, check=True)
    except (OSError, subprocess.CalledProcessError) as error:
        raise GenerateError(f"git rev-parse --local-env-vars failed: {error}") from error
    return frozenset(names.stdout.split())


def _git_env() -> dict[str, str]:
    """The environment without the variables that point git at a repository: a git hook exports GIT_DIR, and
    inheriting it would run every git command below (and those make and go run) against the hook's repository."""
    local = _local_git_vars()
    return {key: value for key, value in os.environ.items() if key not in local}


def run_git(cwd: pathlib.Path, *args: str, stdin: bytes | None = None) -> bytes:
    """git's stdout; a non-zero exit is an error, never an empty result."""
    result = subprocess.run(["git", *args], cwd=cwd, input=stdin, capture_output=True, check=False, env=_git_env())
    if result.returncode:
        raise GenerateError(f"git {' '.join(args)} failed in {cwd}: {result.stderr.decode(errors='replace').strip()}")
    return result.stdout


def check_arguments(name: str, repo: str) -> None:
    if not NAME_PATTERN.fullmatch(name):
        raise GenerateError(f"invalid name {name!r}: use lowercase letters, digits and hyphens, starting with a letter")
    if len(name) > MAX_NAME:
        raise GenerateError(f"name {name!r} is longer than {MAX_NAME} characters (it becomes PostgreSQL identifiers)")
    if name == OLD_NAME:
        raise GenerateError(f"choose a name other than {OLD_NAME!r}")
    if not REPO_PATTERN.fullmatch(repo):
        raise GenerateError(f"invalid repository {repo!r}: use owner/name")


def drop_template_only(path: str, text: str) -> str:
    kept, inside = [], False
    for number, line in enumerate(text.splitlines(keepends=True), 1):
        if BEGIN in line or END in line:
            if (BEGIN in line) == inside:
                raise GenerateError(f"{path}:{number}: unbalanced template-only marker")
            inside = not inside
        elif not inside:
            kept.append(line)
    if inside:
        raise GenerateError(f"{path}: template-only block is never closed")
    return "".join(kept)


def excluded_refs(rel: str, text: str) -> list[str]:
    """The references in text (from the file rel) that resolve to a path the export leaves out."""
    found = []
    for match in PATH_REF.finditer(text):
        lead, name = match.groups()
        # a bare path is read from the repository root, a ./ or ../ one from the referring file's directory
        name = name.rstrip("/")
        resolved = posixpath.normpath(posixpath.join(posixpath.dirname(rel), lead + name) if lead else name)
        if any(f"{resolved}/".startswith(prefix) for prefix in EXCLUDED):
            found.append(match.group(0))
    for match in RELATIVE_REF.finditer(text):
        resolved = posixpath.normpath(posixpath.join(posixpath.dirname(rel), match.group(0)))
        if match.group(0) not in found and any(f"{resolved}/".startswith(prefix) for prefix in EXCLUDED):
            found.append(match.group(0))
    return found


def check_export_attributes(top: pathlib.Path, sha: str) -> None:
    """Refuse when any attribute source git archive reads (the tree's .gitattributes, .git/info/attributes,
    core.attributesFile) sets an export attribute on an exported path."""
    paths = [p for p in run_git(top, "ls-tree", "-r", "-z", "--name-only", sha).split(b"\0")
             # template/ outside derived/ is dropped after the export: its attributes cannot change the output
             if p and (p.decode().startswith(DERIVED) or not p.decode().startswith(EXCLUDED))]
    if not paths:
        return
    fields = run_git(top, "check-attr", "-z", "--stdin", "--source", sha, *EXPORT_ATTRIBUTES,
                     stdin=b"\0".join(paths)).split(b"\0")
    hits = sorted({f"{path.decode()} ({attr.decode()})"
                   for path, attr, value in zip(fields[0::3], fields[1::3], fields[2::3])
                   if value not in (b"unspecified", b"unset")})
    if hits:
        raise GenerateError(f"export attributes would change the output: {', '.join(hits)}")


def export(top: pathlib.Path, sha: str, into: pathlib.Path, *pathspecs: str) -> None:
    """Unpack `git archive` of the pathspecs at sha into the new directory into."""
    # tar.umask=user: the archive's modes follow the user's umask, as files they created would
    archive = run_git(top, "-c", "tar.umask=user", "archive", "--format=tar", sha, "--", *pathspecs)
    into.mkdir()
    try:
        with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
            # Python's recommended filter (the default from 3.14). Defence in depth: git trees hold no absolute or ../
            # paths, and links are refused after the export anyway
            tar.extractall(into, filter="data")
    except tarfile.FilterError as error:
        raise GenerateError(f"the template holds an entry the export refuses: {error}") from error


def materialize(top: pathlib.Path, out: pathlib.Path, name: str, repo: str) -> str:
    """Write the checked output tree of top's HEAD to out (which must not exist). Returns the template commit."""
    sha = run_git(top, "rev-parse", "--verify", "HEAD^{commit}").decode().strip()
    check_export_attributes(top, sha)
    # one export: everything but the plans; the derived files are moved into place, then template/ is dropped
    export(top, sha, out, ".", *(f":!{prefix}" for prefix in EXPORTED_EXCLUDED))
    if not (out / LANDMARK).is_file():
        raise GenerateError(f"{top} does not look like the template: no {LANDMARK} at HEAD")
    derived = out / DERIVED
    if derived.is_dir():
        for source in sorted(p for p in derived.rglob("*") if not p.is_dir() or p.is_symlink()):
            target = out / source.relative_to(derived)
            target.parent.mkdir(parents=True, exist_ok=True)
            os.replace(source, target)  # keeps the derived file's own mode
    for prefix in EXCLUDED:
        shutil.rmtree(out / prefix, ignore_errors=True)

    table = {OLD_REPO: repo, OLD_DB: name.replace("-", "_"), OLD_NAME: name}
    # one pass, longest token first: a replacement is never scanned again, so a repository may contain the name
    pattern = re.compile("|".join(re.escape(t) for t in sorted(table, key=len, reverse=True)))
    problems: list[str] = []
    for file in sorted(out.rglob("*")):
        rel = file.relative_to(out).as_posix()
        problems += [f"{rel}: the path holds {m.group(0)!r} (file names are not rewritten)"
                     for m in FORBIDDEN.finditer(rel)]
        if file.is_symlink():  # a link carries a name or a target the checks below never read
            problems.append(f"{rel}: a symbolic link (not copied; use a regular file)")
            continue
        if not file.is_file():
            continue
        data = file.read_bytes()
        try:
            text = drop_template_only(rel, data.decode("utf-8"))
        except UnicodeDecodeError:  # binary: kept as it is, still checked
            latin = data.decode("latin-1")
            problems += [f"{rel}: still contains {m.group(0)!r}" for m in FORBIDDEN.finditer(latin)]
            problems += [f"{rel}: refers to the excluded {ref!r}" for ref in excluded_refs(rel, latin)]
            continue
        # what the output keeps from the template, the replacements blanked out: they may name the project
        kept = pattern.sub("\0", text)
        problems += [f"{rel}: still contains {m.group(0)!r}" for m in FORBIDDEN.finditer(kept)]
        problems += [f"{rel}: refers to the excluded {ref!r}" for ref in excluded_refs(rel, kept)]
        new = pattern.sub(lambda m: table[m.group(0)], text).encode("utf-8")
        if new != data:
            file.write_bytes(new)
    if problems:
        raise GenerateError("the output is not clean; nothing was written:\n  " + "\n  ".join(problems))
    return sha


def run_tool(cwd: pathlib.Path, *args: str) -> None:
    result = subprocess.run(args, cwd=cwd, capture_output=True, text=True, check=False, env=_git_env())
    if result.returncode:
        output = (result.stdout + result.stderr).strip()
        raise GenerateError(f"{' '.join(args)} failed in {cwd}:\n{output}")


def finish_backend(out: pathlib.Path) -> None:
    """Regenerate the Connect code for the new module path, format and build the backend in out."""
    for tool in ("go", "gofmt", "make"):
        if shutil.which(tool) is None:
            raise GenerateError(f"{tool} not found on PATH (Go version in backend/go.mod)")
    # make gen first installs the output's own pinned, checksum-verified buf (root Makefile, bin/ is ignored)
    run_tool(out / "backend", "make", "--no-print-directory", "gen")
    # a new module path can sort differently against the other imports in its group
    run_tool(out / "backend", "gofmt", "-w", ".")
    run_tool(out / "backend", "go", "build", "./...")


def generate(top: pathlib.Path, dest: pathlib.Path, name: str, repo: str) -> str:
    """Generate into dest (missing or an empty directory). Returns the template commit it was generated from."""
    check_arguments(name, repo)
    if not hasattr(tarfile, "data_filter"):
        raise GenerateError(f"Python {sys.version.split()[0]} lacks tarfile's extraction filters "
                            "(Python 3.12+, 3.11.4+, 3.10.12+, 3.9.17+ or 3.8.17+ has them)")
    dest = dest.absolute()
    if dest.exists() and (not dest.is_dir() or any(dest.iterdir())):
        raise GenerateError(f"{dest} exists and is not an empty directory")
    if not dest.parent.is_dir():
        raise GenerateError(f"{dest.parent} does not exist")
    if run_git(top, "status", "--porcelain", "--untracked-files=normal").strip():
        raise GenerateError(f"{top} has uncommitted or untracked files; commit or remove them first")

    # the project is built in a subdirectory: mkdtemp's own directory is 0700, a plain mkdir follows the umask
    work = pathlib.Path(tempfile.mkdtemp(prefix=f".{dest.name}.", dir=dest.parent))
    try:
        out = work / "project"
        sha = materialize(top, out, name, repo)
        finish_backend(out)
        run_git(out, "init", "-q", "-b", "main")
        run_git(out, "add", "-A")
        message = (
            f"chore: start {name} from the template\n\n"
            f"Quality-Gate: initial commit adds every checker's configuration\n"
            f"Large-Change: initial commit generated from {OLD_NAME}@{sha}\n"
        )
        run_git(out, "commit", "-q", "-F", "-", stdin=message.encode())
        os.rename(out, dest)  # same filesystem; replaces an empty dest in one step
    except OSError as error:
        raise GenerateError(f"generating {dest} failed: {error}") from error
    finally:
        shutil.rmtree(work, ignore_errors=True)
    return sha


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("name", help="lowercase letters, digits and hyphens, e.g. my-project")
    parser.add_argument("dest", type=pathlib.Path, help="new or empty directory")
    parser.add_argument("--repo", help=f"GitHub repository as owner/name (default {DEFAULT_OWNER}/<name>)")
    args = parser.parse_args()
    top = pathlib.Path(__file__).resolve().parent.parent
    try:
        sha = generate(top, args.dest, args.name, args.repo or f"{DEFAULT_OWNER}/{args.name}")
    except GenerateError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1
    print(f"Created {args.dest} from {OLD_NAME}@{sha[:12]} (one commit on main, no remote).")
    print(f"Next: cd {args.dest} && make init-env && make dev; README.md's first push sets up main and dev")
    return 0


if __name__ == "__main__":
    sys.exit(main())
