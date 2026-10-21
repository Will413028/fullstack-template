#!/usr/bin/env python3
"""Compare suppression comments and executed tests with the values locked in scripts/quality-locks.json.

Usage: python3 scripts/check_locks.py [--vitest-report <file>] [--go-test-report <file>]
Run from the repository root. Suppressions are counted in tracked files on every run; a test count is checked when its
report is given: vitest's json reporter for the frontend, `go test -json` (backend/test-report.json from `make test`)
for the backend.

Every count must equal its lock. Suppressions may only go down and executed tests only up: when a count moves the
good way, lower or raise the lock in the same commit; when it moves the bad way, a
Quality-Gate trailer in the commit message says why (scripts/check_commit_policy.py).
"""

import argparse
import json
import pathlib
import re
import subprocess
import sys

LOCKS = pathlib.Path("scripts/quality-locks.json")
# Every way to switch a check off, per language. A new spelling belongs here, or it would slip past the lock.
SUPPRESSIONS = {
    # gosec ignores #nosec (backend/.golangci.yml), but a count here still shows one being added
    "go": (re.compile(r"//\s*nolint\b|#nosec\b"), (".go",)),
    "typescript": (
        re.compile(r"biome-ignore|@ts-expect-error|@ts-ignore|@ts-nocheck|eslint-disable"),
        (".ts", ".tsx", ".js", ".mjs", ".cjs"),
    ),
}


def count_suppressions(language: str) -> int:
    pattern, suffixes = SUPPRESSIONS[language]
    files = subprocess.run(["git", "ls-files", "-z"], check=True, capture_output=True, text=True).stdout.split("\0")
    total = 0
    for name in files:
        if not name.endswith(suffixes) or name == "scripts/check_locks.py":
            continue
        path = pathlib.Path(name)
        if path.is_file():
            total += len(pattern.findall(path.read_text(encoding="utf-8", errors="replace")))
    return total


def count_vitest(report: pathlib.Path) -> int:
    data = json.loads(report.read_text(encoding="utf-8"))
    return data["numPassedTests"] + data["numFailedTests"]


def count_go(report: pathlib.Path) -> int:
    # Tests and subtests that ran to a result, per package; skipped ones do not count.
    ran = set()
    for line in report.read_text(encoding="utf-8").splitlines():
        if not line.startswith("{"):
            continue
        event = json.loads(line)
        if event.get("Test") and event.get("Action") in ("pass", "fail"):
            ran.add((event["Package"], event["Test"]))
    return len(ran)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--vitest-report", type=pathlib.Path)
    parser.add_argument("--go-test-report", type=pathlib.Path)
    args = parser.parse_args()

    locks = json.loads(LOCKS.read_text(encoding="utf-8"))
    counts = {f"suppressions.{lang}": count_suppressions(lang) for lang in SUPPRESSIONS}
    if args.vitest_report:
        counts["tests.frontend"] = count_vitest(args.vitest_report)
    if args.go_test_report:
        counts["tests.backend"] = count_go(args.go_test_report)

    failed = False
    for key, got in counts.items():
        group, name = key.split(".")
        want = locks[group][name]
        if got == want:
            print(f"{key}: {got}")
            continue
        failed = True
        better = got < want if group == "suppressions" else got > want
        if better:
            print(f"{key}: {got}, locked at {want}: set the lock in {LOCKS} to {got}", file=sys.stderr)
        else:
            print(
                f"{key}: {got}, locked at {want}: remove the change, or set the lock to {got} with a Quality-Gate: <why> trailer in the commit",
                file=sys.stderr,
            )
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
