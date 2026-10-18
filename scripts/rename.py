#!/usr/bin/env python3
"""Rename a fresh clone of the template: Docker image, package and database names, branding, and the Go module path.

Usage: python3 scripts/rename.py <name> [--module <go module path>]
Run from the repository root. The module path defaults to github.com/Will413028/<name>/backend. Every import of the
old module path is rewritten and `go build ./...` checks the result. Review the changes with `git diff`.
"""

import argparse
import pathlib
import re
import shutil
import subprocess
import sys

DEFAULT_MODULE = "github.com/Will413028/{name}/backend"
BRAND = "Fullstack Template"
# Files that hold the database, role and image names. After the replacements none of them may still say "fullstack".
STACK_FILES = ("docker-compose.yml", ".env-example", "backend/.env-example", ".github/workflows/ci.yml")
BRAND_FILES = (
    "frontend/src/app/layout.tsx",
    "frontend/messages/en.json",
    "frontend/messages/zh-TW.json",
    "backend/api/openapi.yaml",
    "AGENTS.md",
    "README.md",
)
# Text files under backend/ that may name the module path (imports, depguard rules).
MODULE_FILE_SUFFIXES = (".go", ".yml", ".yaml")
NAME_PATTERN = re.compile(r"[a-z](?:[a-z0-9-]*[a-z0-9])?")


def update(path: pathlib.Path, change) -> bool:
    text = path.read_text(encoding="utf-8")
    new = change(text)
    if new == text:
        return False
    path.write_text(new, encoding="utf-8")
    print(f"Updated {path}")
    return True


def rename_stack(text: str, name: str, db: str) -> str:
    text = text.replace("fullstack-template-backend", f"{name}-backend")
    # fullstack, fullstack_owner, fullstack_app: the database and its roles (unquoted identifiers, no hyphens)
    return re.sub(r"\bfullstack(?=_owner\b|_app\b|\b)", db, text)


def rename_module(backend: pathlib.Path, old: str, new: str) -> list[pathlib.Path]:
    subprocess.run(["go", "mod", "edit", "-module", new], cwd=backend, check=True)
    print(f"Updated {backend / 'go.mod'}")
    # the old path itself or one of its packages, never a longer path that only starts with it
    pattern = re.compile(re.escape(old) + r"(?=[/\"`\s]|$)", re.M)
    changed = []
    for path in sorted(backend.rglob("*")):
        rel = path.relative_to(backend)
        if not path.is_file() or path.suffix not in MODULE_FILE_SUFFIXES or rel.parts[0] == "bin":
            continue
        if update(path, lambda t: pattern.sub(new, t)):
            changed.append(path)
    return changed


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("name", help="lowercase letters, digits and hyphens, e.g. my-project")
    parser.add_argument("--module", help=f"Go module path (default {DEFAULT_MODULE})")
    args = parser.parse_args()

    name = args.name
    if not NAME_PATTERN.fullmatch(name):
        parser.error(f"invalid name {name!r}: use lowercase letters, digits and hyphens, starting with a letter")
    db = name.replace("-", "_")
    module = args.module or DEFAULT_MODULE.format(name=name)
    root = pathlib.Path.cwd()
    backend = root / "backend"
    if not (backend / "go.mod").exists():
        parser.error("backend/go.mod not found; run from the repository root")
    if shutil.which("go") is None:
        parser.error("go not found on PATH (version in backend/go.mod)")
    print(f"Renaming project to {name!r} (Go module {module})...")

    for rel in STACK_FILES:
        path = root / rel
        update(path, lambda t: rename_stack(t, name, db))
        if re.search(r"fullstack", path.read_text(encoding="utf-8"), re.I):
            print(f"error: {rel} still names fullstack; update scripts/rename.py", file=sys.stderr)
            return 1
    update(root / "frontend/package.json", lambda t: t.replace("fullstack-template-frontend", f"{name}-frontend"))
    update(backend / "internal/platform/config/config.go", lambda t: t.replace('"fullstack-template"', f'"{name}"'))
    repo = re.match(r"github\.com/[^/]+/[^/]+", module)
    if repo:
        update(root / "SECURITY.md", lambda t: t.replace("github.com/Will413028/fullstack-template", repo.group()))
    else:
        print("note: the module path is not on github.com; update the advisory link in SECURITY.md by hand")
    for rel in BRAND_FILES:
        update(root / rel, lambda t: t.replace(BRAND, name))

    old_module = subprocess.run(
        ["go", "list", "-m"], cwd=backend, check=True, capture_output=True, text=True
    ).stdout.strip()
    if old_module != module:
        changed = rename_module(backend, old_module, module)
        # a new path can sort differently against the other imports in its group
        go_files = [str(p) for p in changed if p.suffix == ".go"]
        if go_files:
            subprocess.run(["gofmt", "-w", *go_files], check=True)

    print("Checking the backend builds: go build ./...")
    if subprocess.run(["go", "build", "./..."], cwd=backend, check=False).returncode != 0:
        print("error: go build failed after the rename", file=sys.stderr)
        return 1
    print("Done! Review the changes with: git diff")
    print("An existing .env keeps the old database and role names; compare it with .env-example.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
