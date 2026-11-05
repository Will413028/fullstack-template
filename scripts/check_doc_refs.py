#!/usr/bin/env python3
"""Check that every reference to a document's decision or step names that document, which exists and defines it.

Usage: python3 scripts/check_doc_refs.py [root]   (default: the current directory)

Decisions are labelled D1, D2 … and procedures step 1, step 2 … inside the document that makes them, and those
numbers restart in every document. A bare label in a comment therefore points nowhere, or at the wrong decision once
another document reuses the number. So a label is written with its document:

    docs/adr/<file>.md D2, D3        a path, from the repository root
    ADR <slug> D2                    docs/adr/<date>-<slug>.md
    [text](<file>.md) D2             a Markdown link, relative to the file that holds it
    <date>-<slug>.md D2              an ADR's file name: next to the file that holds it, or in docs/adr/

and the document must exist and define the label (a heading, list item or table row that starts with it, bold or
not). A document may use its own labels bare, so inside a document a bare label always means its own. Every docs/
path ending in .md, and every ADR <slug>, must exist too.

Two labels joined by an en dash are a range, and both ends must exist.

Every "step N" counts as a label, numbered procedure comments included: number those "1.", "2." instead.

Checks the files git tracks (every file when root is not a repository), skipping binary files and the path prefixes
listed in .doc-refs-ignore, one per line (# starts a comment).
"""

import bisect
import functools
import pathlib
import re
import subprocess
import sys

IGNORE_FILE = ".doc-refs-ignore"
_D = r"D\d{1,2}(?![\w-]|\.\w)"
_STEP = r"(?:[Ss]teps? |步驟 ?)\d+[a-z]?(?![\w-]|\.\w)"
LABEL = re.compile(rf"(?<![\w#-])(?:{_D}|{_STEP})")
# whitespace within a paragraph: one line break at most, so a reference never reaches into the next block
_GAP = r"[ \t]*(?:\n[ \t]*)?"
_LABELS = rf"(?:{_D}|{_STEP})(?:{_GAP}(?:,|、|\u2013|and|與|和){_GAP}(?:{_D}|{_STEP}))*"
REFERENCE = re.compile(
    r"(?:(?<![\w/.-])(?P<path>(?:docs|\.claude)/[\w./-]*\.md)"
    r"|\]\((?P<link>(?![a-z][a-z0-9+.-]*:)[^)\s#]+\.md)(?:#[^)\s]*)?\)"
    r"|(?<![\w/.-])(?P<dated>\d{4}-\d{2}-\d{2}-[\w.-]+\.md)"
    r"|(?<![\w-])ADR\s+(?P<slug>[a-z0-9]+(?:-[a-z0-9]+)+))"
    rf"(?:`?,?(?![ \t]*\n[ \t]*\n){_GAP}(?<=\s)(?P<labels>{_LABELS}))?"
)


def _files(root: pathlib.Path) -> list[str]:
    if (root / ".git").exists():
        listed = subprocess.run(["git", "ls-files", "-z"], cwd=root, capture_output=True, check=True).stdout
        return [name for name in listed.decode().split("\0") if name]
    skipped = {".git", "node_modules", ".venv"}
    return sorted(p.relative_to(root).as_posix() for p in root.rglob("*") if p.is_file() and not skipped & set(p.parts))


def _ignored(root: pathlib.Path) -> tuple[str, ...]:
    path = root / IGNORE_FILE
    if not path.is_file():
        return ()
    lines = (line.split("#", 1)[0].strip() for line in path.read_text(encoding="utf-8").splitlines())
    return tuple(line for line in lines if line)


def _normal(label: str) -> str:
    """One spelling per label: D2, step 6b."""
    return re.sub(r"^(?:[Ss]teps? |步驟 ?)", "step ", label)


@functools.cache
def _defined(path: pathlib.Path) -> frozenset[str]:
    """The labels a document defines: the first label of a heading, list item or table row."""
    try:
        text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return frozenset()
    start = re.compile(rf"^\s*(?:#+|[-*]|\d+\.|\|)?\s*\**\s*({_D}|{_STEP})", re.MULTILINE)
    return frozenset(_normal(m.group(1)) for m in start.finditer(text))


def _target(root: pathlib.Path, rel: str, match: re.Match[str]) -> tuple[pathlib.Path | None, str]:
    """The document a reference names, and how to call it in a message; None when it names no file."""
    if match["path"]:
        return root / match["path"], match["path"]
    if match["link"]:
        link = match["link"]
        target = root / link.lstrip("/") if link.startswith("/") else (root / rel).parent / link
        return target, match["link"]
    if match["dated"]:
        beside = (root / rel).parent / match["dated"]
        return (beside if beside.is_file() else root / "docs/adr" / match["dated"]), match["dated"]
    slug = match["slug"]
    found = sorted((root / "docs/adr").glob(f"[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]-{slug}.md"))
    return (found[0] if len(found) == 1 else None), f"ADR {slug}"


# A comment leader at the start of a line, blanked so that a reference may wrap onto the next comment line (not in
# Markdown, where # starts a heading)
LEADER = re.compile(r"^[ \t]*(?:#+|//+|/?\*+|--|;+)", re.MULTILINE)


def check_text(root: pathlib.Path, rel: str, text: str) -> list[str]:
    problems: list[tuple[int, str]] = []
    markdown = rel.endswith(".md")
    own = _defined(root / rel) if markdown else frozenset()
    flat = text if markdown else LEADER.sub(lambda m: " " * len(m[0]), text)
    starts = [0] + [m.end() for m in re.finditer("\n", text)]

    def at(offset: int) -> int:
        return bisect.bisect_right(starts, offset)

    covered: list[tuple[int, int]] = []
    for match in REFERENCE.finditer(flat):
        target, name = _target(root, rel, match)
        line = at(match.start())
        where = f"{rel}:{line}"
        if match["labels"]:
            covered.append(match.span("labels"))
        if target is None:
            problems.append((line, f"{where}: {name} names no file docs/adr/<date>-{match['slug']}.md"))
        elif not target.is_file():
            problems.append((line, f"{where}: {name} does not exist"))
        elif match["labels"]:
            defined = _defined(target.resolve())
            problems += [
                (line, f"{where}: {name} does not define {label}")
                for label in LABEL.findall(match["labels"])
                if _normal(label) not in defined
            ]
    for label in LABEL.finditer(flat):
        if any(start <= label.start() < end for start, end in covered) or _normal(label[0]) in own:
            continue
        line = at(label.start())
        message = f"{label[0]!r} names no document (write docs/<path>.md {label[0]} or ADR <slug> {label[0]})"
        problems.append((line, f"{rel}:{line}: {message}"))
    return [message for _, message in sorted(problems, key=lambda p: p[0])]


def check(root: pathlib.Path) -> list[str]:
    _defined.cache_clear()
    ignored = _ignored(root)
    problems: list[str] = []
    for rel in _files(root):
        if rel.startswith(ignored) or not (root / rel).is_file():
            continue
        try:
            text = (root / rel).read_text(encoding="utf-8")
        except UnicodeDecodeError:
            continue
        problems += check_text(root, rel, text)
    return problems


def main() -> int:
    root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else ".").resolve()
    problems = check(root)
    for problem in problems:
        print(problem, file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
