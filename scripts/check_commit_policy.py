#!/usr/bin/env python3
"""Require a reason in the commit message when a commit changes a quality gate or is large.

Usage:
  python3 scripts/check_commit_policy.py --commit-msg <file>   # lefthook commit-msg: the staged changes
  python3 scripts/check_commit_policy.py --range <base>..<head> # CI: every non-merge commit in the range

A commit needs a `Quality-Gate: <reason>` trailer when it edits a checker's configuration (GATE_FILES), or moves a
lock in scripts/quality-locks.json the bad way (more suppressions, fewer tests). Moving a lock the good way needs no
reason. A commit that changes more than MAX_LINES lines, not counting files .gitattributes marks linguist-generated,
needs a `Large-Change: <reason>` trailer: a change that size is hard to review, so it is split or says why it cannot
be. Run from the repository root. In --commit-msg mode the staged changes are compared with HEAD, so an amend is
checked only for what it adds; CI checks the whole commit.
"""

import argparse
import json
import subprocess
import sys

TRAILER = "Quality-Gate"
LARGE_TRAILER = "Large-Change"
MAX_LINES = 1000
LOCKS = "scripts/quality-locks.json"
# Files whose edits can switch a check off or lower a bar. A new checker's configuration belongs here.
GATE_FILES = {
    "backend/.golangci.yml",
    "frontend/.dependency-cruiser.cjs",
    "frontend/biome.json",
    "frontend/knip.config.ts",
    "frontend/tsconfig.json",
    "scripts/check_locks.py",
    "scripts/check_commit_policy.py",
}
GATE_SUFFIXES = ("osv-scanner.toml",)


def git(*args: str) -> str:
    return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout


def show(rev: str, path: str) -> str | None:
    # rev "" is the index. A missing path (new file, first commit) reads as None.
    listed = git("ls-files", "--", path) if rev == "" else git("ls-tree", "--name-only", rev, "--", path)
    return git("show", f"{rev}:{path}") if listed.strip() else None


def trailers(message: str) -> dict[str, str]:
    body = "\n".join(line for line in message.splitlines() if not line.startswith("#"))
    parsed = subprocess.run(
        ["git", "interpret-trailers", "--parse"], input=body, check=True, capture_output=True, text=True
    ).stdout
    found = {}
    for line in parsed.splitlines():
        key, _, value = line.partition(":")
        if value.strip():
            found[key.strip()] = value.strip()
    return found


def worse_locks(old: str | None, new: str | None) -> list[str]:
    if old is None or new is None:
        return []
    before, after = json.loads(old), json.loads(new)
    moved = []
    for group, names in after.items():
        for name, value in names.items():
            was = before.get(group, {}).get(name)
            if was is None:
                continue
            if (group == "suppressions" and value > was) or (group == "tests" and value < was):
                moved.append(f"{group}.{name} {was} -> {value}")
    return moved


def changed_lines(numstat: str, new_rev: str) -> int:
    counts = {}
    for line in numstat.splitlines():
        added, deleted, path = line.split("\t", 2)
        if added != "-":  # binary
            counts[path] = int(added) + int(deleted)
    if not counts:
        return 0
    source = ["--cached"] if new_rev == "" else ["--source", new_rev]
    attrs = git("check-attr", *source, "-z", "linguist-generated", "--", *counts).split("\0")
    generated = {attrs[i] for i in range(0, len(attrs) - 2, 3) if attrs[i + 2] in ("set", "true")}
    return sum(n for path, n in counts.items() if path not in generated)


def violations(changed: list[str], old_rev: str | None, new_rev: str, message: str, numstat: str) -> list[str]:
    """Each problem as '<what the commit does>: add a '<trailer>: <why>' trailer'."""
    found = trailers(message)
    reasons = []
    if TRAILER not in found:
        reasons += [f"edits {path}" for path in changed if path in GATE_FILES or path.endswith(GATE_SUFFIXES)]
        if LOCKS in changed:
            old = show(old_rev, LOCKS) if old_rev is not None else None
            reasons += [f"moves the lock {m}" for m in worse_locks(old, show(new_rev, LOCKS))]
    problems = [f"{r}: add a '{TRAILER}: <why>' trailer" for r in reasons]
    if LARGE_TRAILER not in found and (lines := changed_lines(numstat, new_rev)) > MAX_LINES:
        problems.append(
            f"changes {lines} lines, over {MAX_LINES} without generated files: "
            f"split it, or add a '{LARGE_TRAILER}: <why>' trailer"
        )
    return problems


def check_staged(message_file: str) -> list[str]:
    has_head = subprocess.run(["git", "rev-parse", "--verify", "-q", "HEAD"], capture_output=True).returncode == 0
    changed = git("diff", "--cached", "--name-only").splitlines()
    with open(message_file, encoding="utf-8") as f:
        message = f.read()
    numstat = git("diff", "--cached", "--numstat", "--no-renames")
    return violations(changed, "HEAD" if has_head else None, "", message, numstat)


def check_range(rev_range: str) -> list[str]:
    failures = []
    for commit in git("rev-list", "--no-merges", "--reverse", rev_range).split():
        parents = git("rev-list", "--parents", "-n1", commit).split()[1:]
        old_rev = parents[0] if parents else None
        old_tree = old_rev or git("hash-object", "-t", "tree", "/dev/null").strip()
        changed = git("diff", "--name-only", old_tree, commit).splitlines()
        numstat = git("diff", "--numstat", "--no-renames", old_tree, commit)
        found = violations(changed, old_rev, commit, git("log", "-1", "--format=%B", commit), numstat)
        failures += [f"{commit[:12]} {reason}" for reason in found]
    return failures


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--commit-msg", metavar="FILE")
    mode.add_argument("--range", metavar="BASE..HEAD")
    args = parser.parse_args()

    failures = check_staged(args.commit_msg) if args.commit_msg else check_range(args.range)
    for failure in failures:
        print(failure, file=sys.stderr)
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
