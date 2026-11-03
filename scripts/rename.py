#!/usr/bin/env python3
"""Rename a fresh clone of the template: Docker image, package and database names, branding, and the Go module path.

Usage: python3 scripts/rename.py <name> [--module <go module path>]
Run from the repository root. The module path defaults to github.com/Will413028/<name>/backend. Every import of the
old module path is rewritten, the generated Connect code is regenerated (`make -C backend gen`, which downloads the
pinned buf), and `go build ./...` checks the result. Review the changes with `git diff`.

The proto packages keep their root `app` (docs/adr/2026-11-03-api-contract-protobuf-connect.md D5), so the contract
in api/ is not renamed.
"""

import argparse
import pathlib
import re
import shutil
import subprocess
import sys

DEFAULT_MODULE = "github.com/Will413028/{name}/backend"
BRAND = "template-go-nextjs"
OLD_NAME = "template-go-nextjs"
OLD_DB = "template_go_nextjs"
# Files that hold the database, role and image names. After the replacements none of them may still name OLD_DB.
STACK_FILES = ("docker-compose.yml", ".env-example", "backend/.env-example", ".github/workflows/ci.yml")
BRAND_FILES = (
    "frontend/src/app/layout.tsx",
    "frontend/messages/en.json",
    "frontend/messages/zh-TW.json",
    "backend/api/openapi.yaml",
    "AGENTS.md",
    "README.md",
)
# Text files under backend/ that may name the module path (imports, depguard rules, buf.gen.yaml).
MODULE_FILE_SUFFIXES = (".go", ".yml", ".yaml")
# Generated Connect code (backend/Makefile GEN_DIRS): protoc-gen-go stores go_package, the module path included, with
# a length prefix in each file's raw descriptor, so a text replacement would make it panic at init. It is regenerated.
GENERATED_DIRS = (pathlib.PurePath("internal/gen"), pathlib.PurePath("internal/httpapi/rpc/testdata/gen"))
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
    text = text.replace(f"{OLD_NAME}-backend", f"{name}-backend")
    # OLD_DB, OLD_DB_owner, OLD_DB_app: the database and its roles (unquoted identifiers, no hyphens)
    return re.sub(rf"\b{OLD_DB}(?=_owner\b|_app\b|\b)", db, text)


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
        if any(rel.is_relative_to(d) for d in GENERATED_DIRS):
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
        if OLD_DB in path.read_text(encoding="utf-8"):
            print(f"error: {rel} still names {OLD_DB}; update scripts/rename.py", file=sys.stderr)
            return 1
    update(root / "frontend/package.json", lambda t: t.replace(f"{OLD_NAME}-frontend", f"{name}-frontend"))
    update(backend / "internal/platform/config/config.go", lambda t: t.replace(f'"{OLD_NAME}"', f'"{name}"'))
    repo = re.match(r"github\.com/[^/]+/[^/]+", module)
    if repo:
        update(root / "SECURITY.md", lambda t: t.replace(f"github.com/Will413028/{OLD_NAME}", repo.group()))
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
        print("Regenerating the Connect code for the new module path: make -C backend gen")
        if subprocess.run(["make", "-C", "backend", "gen"], check=False).returncode != 0:
            print("error: make -C backend gen failed after the rename", file=sys.stderr)
            return 1

    print("Checking the backend builds: go build ./...")
    if subprocess.run(["go", "build", "./..."], cwd=backend, check=False).returncode != 0:
        print("error: go build failed after the rename", file=sys.stderr)
        return 1
    print("Done! Review the changes with: git diff")
    print("An existing .env keeps the old database and role names; compare it with .env-example.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
