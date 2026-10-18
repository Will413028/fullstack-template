#!/usr/bin/env python3
"""One-off check for the Go rewrite inventory (removed with the plan at close-out).

1. Every table row in inventory.md has a non-empty last column (去向/處置).
2. Every file the consumer search finds outside backend/ is listed in section F.
3. With --after-rewrite: no Python-backend residue anywhere in the repo,
   backend/ included, except the line-level allowlist below.

Usage: python3 check_inventory.py [--after-rewrite]
"""

import pathlib
import re
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
REPO = HERE.parents[2]
INVENTORY = HERE / "inventory.md"

PATTERN = r"backend|python|uv |pytest|ruff|alembic|diff-cover|fastapi|sqlalchemy|pydantic"
GLOBS = ["!backend/**", "!frontend/node_modules/**", "!**/pnpm-lock.yaml", "!docs/plans/**", "!.git/**"]

# Python-backend words that must be gone after the rewrite. "backend" is a service
# name and stays; diff-cover stays as the coverage tool.
PY_PATTERN = r"python|\buv\b|pytest|ruff|alembic|fastapi|sqlalchemy|pydantic|pyproject|\.venv"
RESIDUE_GLOBS = ["!frontend/node_modules/**", "!**/pnpm-lock.yaml", "!docs/plans/**", "!.git/**"]
# (path regex, line regex) pairs that may legitimately match PY_PATTERN.
ALLOWED_LINES = [
    (r"^scripts/[^/]+\.py$", r"python"),  # repo tooling stays Python (plan P11)
    (r"^docs/adr/", r"."),  # decision records keep history
    (r"^docs/deploy-vps\.md$", r"Python 3（`make init-env`"),  # host requirement for scripts/
    (r"^\.github/workflows/ci\.yml$", r"actions/setup-python|pipx run diff-cover"),
    (r"^Makefile$", r"python3? scripts/"),  # rename / init_env targets
    (r"^README\.md$", r"python3? scripts/|^- Python 3 "),  # host requirement for scripts/
    # standard-library one-liners in backend tooling (Atlas pin age, coverage summary), not a Python backend
    (r"^backend/(Makefile|scripts/coverage-report\.sh)$", r"python3 -I "),
]


def rg(pattern: str, globs: list[str], *, lines: bool = False) -> list[str]:
    cmd = ["rg", "-n" if lines else "-l", "-i", "--hidden", pattern, "."]
    for g in globs:
        cmd += ["-g", g]
    out = subprocess.run(cmd, cwd=REPO, capture_output=True, text=True, check=False)
    return sorted(line.removeprefix("./") for line in out.stdout.splitlines() if line)


def check_rows(text: str) -> list[str]:
    errors = []
    for n, line in enumerate(text.splitlines(), 1):
        if not line.startswith("| ") or set(line) <= set("|- "):
            continue
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        if cells[0] == "#":
            continue
        if not cells[-1]:
            errors.append(f"inventory.md:{n}: empty 去向/處置")
    return errors


def check_consumers(text: str) -> list[str]:
    section = text.split("## F.", 1)[1]
    listed = set(re.findall(r"`([^`\s:]+?)(?::[\d,\-]+)?`", section))
    return [f"consumer not in section F: {p}" for p in rg(PATTERN, GLOBS) if p not in listed]


def allowed(path: str, line: str) -> bool:
    return any(re.search(pp, path) and re.search(lp, line, re.I) for pp, lp in ALLOWED_LINES)


def check_after() -> list[str]:
    errors = []
    for hit in rg(PY_PATTERN, RESIDUE_GLOBS, lines=True):
        path, _, rest = hit.partition(":")
        _, _, line = rest.partition(":")
        if not allowed(path, line):
            errors.append(f"python residue: {hit}")
    # Old-contract residue: the cross-origin API URL and the verb-style auth paths.
    old_contract = r"NEXT_PUBLIC_API_URL|/auth/(login|logout|register|refresh|me)\b|access_token|refresh_token"
    for hit in rg(old_contract, RESIDUE_GLOBS + ["!docs/adr/**"], lines=True):
        errors.append(f"old contract residue: {hit}")
    for leftover in ("backend/pyproject.toml", "backend/uv.lock", "backend/alembic.ini", "backend/src"):
        if (REPO / leftover).exists():
            errors.append(f"still exists: {leftover}")
    return errors


def main() -> int:
    text = INVENTORY.read_text(encoding="utf-8")
    errors = check_rows(text) + check_consumers(text)
    if "--after-rewrite" in sys.argv:
        errors += check_after()
    for e in errors:
        print(e)
    print("OK" if not errors else f"{len(errors)} problem(s)")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
