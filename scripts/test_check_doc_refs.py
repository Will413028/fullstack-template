"""Tests for check_doc_refs.py. Run: python3 -m unittest discover -s scripts -p 'test_*.py'"""

import importlib
import pathlib
import subprocess
import sys
import tempfile
import unittest

SCRIPTS = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPTS))
check_doc_refs = importlib.import_module("check_doc_refs")

ADR = """# Auth baseline

- **D1 Sessions**: opaque.
- **D2 Throttling**: per address.
"""
RULES = """# Adding a module

## Step 1 — the table
## Step 2 — the routes
Then step 1 again.
"""


class CheckDocRefsTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)
        self.write("docs/adr/2026-10-18-auth-baseline.md", ADR)
        self.write("docs/rules/add-module.md", RULES)

    def write(self, rel: str, text: str) -> None:
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def problems(self, rel: str, text: str) -> list[str]:
        self.write(rel, text)
        return check_doc_refs.check(self.root)

    def test_qualified_references_to_defined_labels_pass(self):
        self.assertEqual(
            self.problems(
                "src/a.py",
                "# opaque (docs/adr/2026-10-18-auth-baseline.md D1, D2)\n"
                "# throttled (ADR auth-baseline D2); see docs/rules/add-module.md step 2\n",
            ),
            [],
        )

    def test_a_document_uses_its_own_labels_bare(self):
        self.assertEqual(check_doc_refs.check(self.root), [])

    def test_a_markdown_link_qualifies_labels_relative_to_the_file(self):
        self.assertEqual(self.problems("docs/adr/other.md", "[auth](2026-10-18-auth-baseline.md) D2\n"), [])

    def test_a_dated_file_name_names_an_adr(self):
        self.assertEqual(self.problems("src/a.py", "# 2026-10-18-auth-baseline.md D1\n"), [])
        self.assertEqual(
            self.problems("docs/adr/b.md", "supersedes 2026-10-18-go-backend.md D2\n"),
            ["docs/adr/b.md:1: 2026-10-18-go-backend.md does not exist"],
        )

    def test_a_reference_may_wrap_across_comment_lines(self):
        self.assertEqual(
            self.problems("src/a.py", "# opaque (docs/adr/2026-10-18-auth-baseline.md\n# D1); next line D2\nx = 1\n"),
            ["src/a.py:2: 'D2' names no document (write docs/<path>.md D2 or ADR <slug> D2)"],
        )

    def test_a_range_and_a_quoted_path_qualify_labels(self):
        self.assertEqual(self.problems("src/a.md", "the `docs/adr/2026-10-18-auth-baseline.md` D1\u2013D2\n"), [])

    def test_external_links_and_anchors(self):
        self.assertEqual(
            self.problems(
                "docs/rules/s.md",
                "[x](https://example.com/README.md) and [y](../adr/2026-10-18-auth-baseline.md#d1) D1\n",
            ),
            [],
        )

    def test_a_reference_does_not_reach_past_a_blank_line(self):
        self.assertEqual(
            self.problems("docs/rules/r.md", "See docs/adr/2026-10-18-auth-baseline.md\n\n## Step 1 — table\n"), []
        )

    def test_names_that_only_contain_a_label_pass(self):
        self.assertEqual(
            self.problems("src/c.ts", 'uses D3.js charts; color = "#D3"; end of D1.\n'),
            ["src/c.ts:1: 'D1' names no document (write docs/<path>.md D1 or ADR <slug> D1)"],
        )

    def test_an_adr_slug_without_labels_must_exist(self):
        self.assertEqual(
            self.problems("src/a.py", "# (ADR auth-baselin) and the ADR gate\n"),
            ["src/a.py:1: ADR auth-baselin names no file docs/adr/<date>-auth-baselin.md"],
        )

    def test_a_path_with_a_colon_is_reported(self):
        self.assertEqual(
            self.problems("x:y/c.py", "# D9\n"),
            ["x:y/c.py:1: 'D9' names no document (write docs/<path>.md D9 or ADR <slug> D9)"],
        )

    def test_a_bare_label_outside_its_document_fails(self):
        self.assertEqual(
            self.problems("src/a.py", "x = 1  # the contract's models (D8)\n"),
            ["src/a.py:1: 'D8' names no document (write docs/<path>.md D8 or ADR <slug> D8)"],
        )

    def test_a_bare_step_outside_its_document_fails(self):
        self.assertEqual(
            self.problems(".gitignore", "# passwords the step 0 spikes generated\n"),
            [".gitignore:1: 'step 0' names no document (write docs/<path>.md step 0 or ADR <slug> step 0)"],
        )

    def test_a_label_the_document_does_not_define_fails(self):
        self.assertEqual(
            self.problems("src/a.py", "# (docs/adr/2026-10-18-auth-baseline.md D2, D9)\n"),
            ["src/a.py:1: docs/adr/2026-10-18-auth-baseline.md does not define D9"],
        )

    def test_a_missing_document_fails(self):
        self.assertEqual(
            self.problems("src/a.py", "# see docs/notes/2026-11-03-x.md D6\n"),
            ["src/a.py:1: docs/notes/2026-11-03-x.md does not exist"],
        )

    def test_an_adr_slug_must_name_one_adr(self):
        self.assertEqual(
            self.problems("src/a.py", "# (ADR api-and-auth D6)\n"),
            ["src/a.py:1: ADR api-and-auth names no file docs/adr/<date>-api-and-auth.md"],
        )

    def test_identifiers_that_only_look_like_labels_pass(self):
        self.assertEqual(self.problems("src/a.ts", "const D2x = 1; const xD2 = 'ID2'; v = D2_KEY;\n"), [])

    def test_binary_files_are_skipped(self):
        (self.root / "logo.bin").write_bytes(b"\xff\xfe D8 \x00")
        self.assertEqual(check_doc_refs.check(self.root), [])

    def test_a_git_repository_checks_tracked_files_only(self):
        self.write("ignored/a.py", "# (D8)\n")
        self.write(".gitignore", "ignored/\n")
        env = {"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1", "PATH": "/usr/bin:/bin"}
        subprocess.run(["git", "init", "-q"], cwd=self.root, check=True, env=env)
        subprocess.run(["git", "add", "-A"], cwd=self.root, check=True, env=env)
        self.assertEqual(check_doc_refs.check(self.root), [])


if __name__ == "__main__":
    unittest.main()
