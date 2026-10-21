"""Tests for check_commit_policy.py against throwaway git repositories. Run: python3 -m unittest discover scripts"""

import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parent / "check_commit_policy.py"
LOCKS = {"suppressions": {"go": 2}, "tests": {"backend": 10}}


class CommitPolicyTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.repo = pathlib.Path(self.tmp.name)
        # Hooks export GIT_DIR; the script must still find the repository it runs in.
        self.env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
        self.env |= {"GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1"}
        self.git("init", "-q", "-b", "main")
        self.git("config", "user.email", "test@example.com")
        self.git("config", "user.name", "test")
        self.write("scripts/quality-locks.json", json.dumps(LOCKS))
        self.write("backend/main.go", "package main\n")
        self.commit("chore: start")
        self.base = self.git("rev-parse", "HEAD").strip()

    def tearDown(self):
        self.tmp.cleanup()

    def git(self, *args):
        return subprocess.run(
            ["git", *args], cwd=self.repo, env=self.env, check=True, capture_output=True, text=True
        ).stdout

    def write(self, path, text):
        target = self.repo / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text, encoding="utf-8")
        self.git("add", path)

    def commit(self, message):
        self.git("commit", "-q", "--no-verify", "-m", message)

    def run_script(self, *args):
        return subprocess.run(
            [sys.executable, str(SCRIPT), *args], cwd=self.repo, env=self.env, capture_output=True, text=True
        )

    def check_range(self):
        return self.run_script("--range", f"{self.base}..HEAD")

    def check_staged(self, message):
        msg = self.repo / ".git" / "COMMIT_EDITMSG"
        msg.write_text(message, encoding="utf-8")
        return self.run_script("--commit-msg", str(msg))

    def test_gate_file_edit_needs_trailer(self):
        self.write("backend/.golangci.yml", "version: 2\n")
        self.commit("ci: loosen lint")
        result = self.check_range()
        self.assertEqual(result.returncode, 1)
        self.assertIn("edits backend/.golangci.yml", result.stderr)

    def test_gate_file_edit_with_trailer_passes(self):
        self.write("frontend/biome.json", "{}\n")
        self.commit("ci: tighten lint\n\nQuality-Gate: turn on noSkippedTests")
        self.assertEqual(self.check_range().returncode, 0, self.check_range().stderr)

    def test_nested_osv_exceptions_need_trailer(self):
        self.write("backend/tools/osv-scanner.toml", "[[IgnoredVulns]]\n")
        self.commit("chore: ignore a vuln")
        self.assertIn("edits backend/tools/osv-scanner.toml", self.check_range().stderr)

    def test_lock_moved_bad_way_needs_trailer(self):
        locks = json.loads(json.dumps(LOCKS))
        locks["suppressions"]["go"] = 3
        locks["tests"]["backend"] = 9
        self.write("scripts/quality-locks.json", json.dumps(locks))
        self.commit("chore: more nolint")
        stderr = self.check_range().stderr
        self.assertIn("moves the lock suppressions.go 2 -> 3", stderr)
        self.assertIn("moves the lock tests.backend 10 -> 9", stderr)

    def test_lock_moved_good_way_passes(self):
        locks = json.loads(json.dumps(LOCKS))
        locks["suppressions"]["go"] = 1
        locks["tests"]["backend"] = 11
        self.write("scripts/quality-locks.json", json.dumps(locks))
        self.commit("test: one more test")
        self.assertEqual(self.check_range().returncode, 0)

    def test_ordinary_change_passes(self):
        self.write("backend/main.go", "package main\n\nfunc main() {}\n")
        self.commit("feat: main")
        self.assertEqual(self.check_range().returncode, 0)

    def test_large_change_needs_trailer(self):
        self.write("backend/big.go", "package main\n" + "// line\n" * 1000)
        self.commit("feat: big")
        result = self.check_range()
        self.assertEqual(result.returncode, 1)
        self.assertIn("changes 1001 lines, over 1000", result.stderr)

    def test_large_change_with_trailer_passes(self):
        self.write("backend/big.go", "package main\n" + "// line\n" * 1000)
        self.commit("feat: big\n\nLarge-Change: one generated table, reviewed as a whole")
        self.assertEqual(self.check_range().returncode, 0)

    def test_generated_files_do_not_count(self):
        self.write(".gitattributes", "*.sum linguist-generated=true\n")
        self.write("backend/go.sum", "h1\n" * 5000)
        self.commit("build: bump deps")
        self.assertEqual(self.check_range().returncode, 0, self.check_range().stderr)

    def test_commit_msg_mode_counts_staged_lines(self):
        self.write("backend/big.go", "package main\n" + "// line\n" * 1000)
        self.assertIn("changes 1001 lines", self.check_staged("feat: big\n").stderr)

    def test_commit_msg_mode_checks_staged_changes(self):
        self.write("frontend/tsconfig.json", "{}\n")
        result = self.check_staged("chore: relax ts\n")
        self.assertEqual(result.returncode, 1)
        self.assertIn("edits frontend/tsconfig.json", result.stderr)
        # a commented-out trailer does not count
        self.assertEqual(self.check_staged("chore: relax ts\n\n# Quality-Gate: no\n").returncode, 1)
        self.assertEqual(self.check_staged("chore: relax ts\n\nQuality-Gate: needed for X\n").returncode, 0)


if __name__ == "__main__":
    unittest.main()
