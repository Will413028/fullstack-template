#!/usr/bin/env python3
"""One-off checks for docs/plans/2026-11-03-connect-contract.md (called from the plan's step acceptance).

  python3 check-plan.py            inventory rows all have a Destination; every step listed in the plan has the fields
  python3 check-plan.py tests [GLOB]  also: every snapshot test in GO_DIRS and every frontend test file is in tests-map.md
                                      (GLOB limits it to matching files, e.g. 'backend/internal/httpapi/items_test.go')
"""
import fnmatch
import pathlib
import re
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
REPO = HERE.parents[2]
PLAN = HERE.parent / "2026-11-03-connect-contract.md"
SNAPSHOT = "a83acc9"
FIELDS = ("  - 範圍", "消費端：", "不能動：", "驗收：", "停止條件：")


def inventory() -> list[str]:
    errors = []
    for n, line in enumerate((HERE / "inventory.md").read_text().splitlines(), 1):
        m = re.match(r"^\| ([OBTFRD]\d+) \|", line)
        if m and line.rstrip().endswith("| |"):
            errors.append(f"inventory.md:{n}: {m.group(1)} has no Destination")
    return errors


def steps() -> list[str]:
    errors = []
    text = PLAN.read_text()
    body = text.split("## 步驟", 1)[1].split("\n## ", 1)[0]
    for block in re.split(r"\n(?=- \[[ x]\] \*\*)", body):
        head = re.match(r"- \[[ x]\] \*\*([0-9a-z.]+)", block)
        if not head or "收尾" in block.splitlines()[0]:
            continue
        if "被擋於：" not in block.splitlines()[0]:
            errors.append(f"step {head.group(1)}: missing 被擋於")
        for f in FIELDS:
            if f not in block:
                errors.append(f"step {head.group(1)}: missing {f}")
    return errors


# Tests that the migration rewrites: every top-level Go test in these packages and every frontend test file
# at the snapshot must appear in tests-map.md (as its own word) with a new test or a retirement reason.
GO_DIRS = ("backend/internal/httpapi/", "backend/cmd/app/")


def snapshot_tests() -> dict[tuple[str, str], str]:
    """(file, name) -> name for every test the map must cover; TestMain is setup, not a test."""
    out = {}
    files = subprocess.run(["git", "-C", str(REPO), "ls-tree", "-r", "--name-only", SNAPSHOT],
                           capture_output=True, text=True, check=True).stdout.split()
    for f in files:
        if f.startswith(GO_DIRS) and f.endswith("_test.go"):
            src = subprocess.run(["git", "-C", str(REPO), "show", f"{SNAPSHOT}:{f}"],
                                 capture_output=True, text=True, check=True).stdout
            for name in re.findall(r"^func (Test[A-Za-z0-9_]+)\(", src, re.M):
                if name != "TestMain":
                    out[(f, name)] = name
        elif f.startswith("frontend/src/") and re.search(r"\.test\.tsx?$", f):
            out[(f, f)] = f
    return out


def tests(glob: str | None) -> list[str]:
    mapped = (HERE / "tests-map.md").read_text() if (HERE / "tests-map.md").exists() else ""
    words = set(re.findall(r"[A-Za-z0-9_./\[\]-]+", mapped))
    want = {k: n for k, n in snapshot_tests().items() if glob is None or fnmatch.fnmatch(k[0], glob)}
    if not want:
        return [f"no snapshot tests match {glob!r}"]
    return [f"tests-map.md: {n} ({k[0]}) not listed" for k, n in sorted(want.items()) if n not in words]


if __name__ == "__main__":
    args = sys.argv[1:]
    errs = inventory() + steps() + (tests(args[1] if len(args) > 1 else None) if args[:1] == ["tests"] else [])
    print("\n".join(errs) or "ok")
    sys.exit(1 if errs else 0)
