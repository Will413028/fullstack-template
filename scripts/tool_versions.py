#!/usr/bin/env python3
"""Keep the release binaries pinned in Makefiles (golangci-lint, hadolint, Atlas) current, with verified checksums.

Dependabot reads manifests, not Makefiles, so these pins would otherwise age unnoticed. Tools that ship as packages
are pinned where Dependabot looks instead (backend/tools: go.mod, package.json, requirements.txt).

Usage (from the repository root):
  python3 scripts/tool_versions.py verify          # offline: Atlas pins in backend/Makefile and backend/Dockerfile agree
  python3 scripts/tool_versions.py check           # print pins older than the newest release past the cooldown;
                                                   # exit 10 when any is, 1 when a source cannot be read
  python3 scripts/tool_versions.py bump <tool> [--version V]
                                                   # rewrite version and checksums from the release's checksum files

Releases younger than COOLDOWN_DAYS are ignored, the same cooldown as Dependabot (.github/dependabot.yml).
"""

import argparse
import datetime as dt
import email.utils
import json
import os
import pathlib
import re
import sys
import urllib.error
import urllib.request

COOLDOWN_DAYS = 7
# check's exit status when a pin is behind, so a caller can tell it from a failure (1).
OUTDATED = 10
BACKEND_MAKEFILE = pathlib.Path("backend/Makefile")
ROOT_MAKEFILE = pathlib.Path("Makefile")
DOCKERFILE = pathlib.Path("backend/Dockerfile")
ATLAS_CDN = "https://release.ariga.io/atlas"


def http_get(url: str, method: str = "GET") -> tuple[int, dict[str, str], bytes]:
    req = urllib.request.Request(url, method=method, headers={"User-Agent": "tool-versions"})
    if url.startswith("https://api.github.com/") and os.environ.get("GITHUB_TOKEN"):
        req.add_header("Authorization", "Bearer " + os.environ["GITHUB_TOKEN"])
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, dict(resp.headers), resp.read()
    except urllib.error.HTTPError as err:
        return err.code, dict(err.headers), b""


def make_var(text: str, name: str) -> str:
    m = re.search(rf"^{re.escape(name)}\s*:=\s*(\S+)", text, re.M)
    if not m:
        raise SystemExit(f"no {name} := … line")
    return m.group(1)


def set_make_var(text: str, name: str, value: str) -> str:
    new, n = re.subn(rf"^({re.escape(name)}\s*:=\s*)\S+", lambda m: m.group(1) + value, text, flags=re.M)
    if n != 1:
        raise SystemExit(f"expected one {name} := … line, found {n}")
    return new


# Each tool: where its version and checksums live, and how a release's checksums are published.
GOLANGCI_PLATFORMS = ["darwin-arm64", "linux-amd64", "linux-arm64"]
HADOLINT_PLATFORMS = ["macos-arm64", "linux-x86_64", "linux-arm64"]
ATLAS_PLATFORMS = ["darwin-arm64", "linux-amd64", "linux-arm64"]


def github_latest(repo: str, cooldown: dt.date) -> tuple[str, dt.date]:
    status, _, body = http_get(f"https://api.github.com/repos/{repo}/releases?per_page=30")
    if status != 200:
        raise SystemExit(f"{repo}: GitHub releases returned {status}")
    for rel in json.loads(body):
        published = dt.date.fromisoformat(rel["published_at"][:10])
        if not rel["draft"] and not rel["prerelease"] and published <= cooldown:
            return rel["tag_name"].removeprefix("v"), published
    raise SystemExit(f"{repo}: no release older than the cooldown")


def checksum_lines(text: str) -> dict[str, str]:
    """Parse `<sha256>  <file>` or `<sha256> *<file>` lines."""
    sums = {}
    for line in text.splitlines():
        parts = line.split()
        if len(parts) == 2 and re.fullmatch(r"[0-9a-f]{64}", parts[0]):
            sums[parts[1].lstrip("*")] = parts[0]
    return sums


def fetch_text(url: str) -> str:
    status, _, body = http_get(url)
    if status != 200:
        raise SystemExit(f"{url}: {status}")
    return body.decode()


def atlas_released(version: str) -> dt.date | None:
    """The CDN's Last-Modified for the linux-amd64 binary, or None when the version does not exist (403/404)."""
    status, headers, _ = http_get(f"{ATLAS_CDN}/atlas-linux-amd64-v{version}", method="HEAD")
    if status != 200:
        return None
    return email.utils.parsedate_to_datetime(headers["Last-Modified"]).date()


def atlas_latest(current: str, cooldown: dt.date) -> tuple[str, dt.date]:
    """Atlas tags GitHub releases only for minors; patch builds exist only on its CDN, so probe it: the next patches
    of the current minor, then the next minor and major."""
    major, minor, patch = (int(x) for x in current.split("."))
    best = (current, atlas_released(current) or dt.date.min)
    candidates = [(major, minor, patch + i) for i in range(1, 20)] + [(major, minor + 1, 0), (major + 1, 0, 0)]
    for version in (".".join(map(str, c)) for c in candidates):
        released = atlas_released(version)
        if released is None:
            continue
        if released <= cooldown:
            best = (version, released)
    return best


TOOLS = {
    "golangci-lint": {"file": BACKEND_MAKEFILE, "version_var": "GOLANGCI_LINT_VERSION"},
    "hadolint": {"file": ROOT_MAKEFILE, "version_var": "HADOLINT_VERSION"},
    "atlas": {"file": BACKEND_MAKEFILE, "version_var": "ATLAS_VERSION"},
}


def pinned(tool: str) -> str:
    spec = TOOLS[tool]
    return make_var(spec["file"].read_text(), spec["version_var"]).removeprefix("v")


def latest(tool: str, cooldown: dt.date) -> tuple[str, dt.date]:
    if tool == "golangci-lint":
        return github_latest("golangci/golangci-lint", cooldown)
    if tool == "hadolint":
        return github_latest("hadolint/hadolint", cooldown)
    return atlas_latest(pinned("atlas"), cooldown)


def new_checksums(tool: str, version: str) -> dict[str, str]:
    """Makefile variable -> checksum for the version, from the release's published checksum files."""
    if tool == "golangci-lint":
        base = f"https://github.com/golangci/golangci-lint/releases/download/v{version}"
        sums = checksum_lines(fetch_text(f"{base}/golangci-lint-{version}-checksums.txt"))
        return {f"GOLANGCI_LINT_SHA256_{p}": sums[f"golangci-lint-{version}-{p}.tar.gz"] for p in GOLANGCI_PLATFORMS}
    if tool == "hadolint":
        sums = checksum_lines(
            fetch_text(f"https://github.com/hadolint/hadolint/releases/download/v{version}/checksums.sha256")
        )
        return {f"HADOLINT_SHA256_{p}": sums[f"hadolint-{p}"] for p in HADOLINT_PLATFORMS}
    out = {}
    for build, prefix in (("atlas", "ATLAS_SHA256"), ("atlas-community", "ATLAS_COMMUNITY_SHA256")):
        for p in ATLAS_PLATFORMS:
            out[f"{prefix}_{p}"] = fetch_text(f"{ATLAS_CDN}/{build}-{p}-v{version}.sha256").split()[0]
    return out


def rewrite_dockerfile_atlas(text: str, version: str, amd64: str, arm64: str) -> str:
    text, n = re.subn(r"^(ARG ATLAS_VERSION=)v\S+", rf"\g<1>v{version}", text, flags=re.M)
    text, a = re.subn(r"(amd64\) sum=)[0-9a-f]{64}", rf"\g<1>{amd64}", text)
    text, b = re.subn(r"(arm64\) sum=)[0-9a-f]{64}", rf"\g<1>{arm64}", text)
    if (n, a, b) != (1, 1, 1):
        raise SystemExit("backend/Dockerfile: expected one ATLAS_VERSION ARG and one amd64/arm64 sum")
    return text


def atlas_mismatches(makefile: str, dockerfile: str) -> list[str]:
    """The image applies migrations with the community build that backend/Makefile pins for the tests."""
    problems = []
    version = make_var(makefile, "ATLAS_VERSION")
    arg = re.search(r"^ARG ATLAS_VERSION=(\S+)", dockerfile, re.M)
    if not arg or arg.group(1) != version:
        problems.append(f"ATLAS_VERSION: Makefile {version}, Dockerfile {arg.group(1) if arg else 'missing'}")
    for arch in ("amd64", "arm64"):
        want = make_var(makefile, f"ATLAS_COMMUNITY_SHA256_linux-{arch}")
        got = re.search(rf"{arch}\) sum=([0-9a-f]{{64}})", dockerfile)
        if not got or got.group(1) != want:
            problems.append(f"Atlas community linux-{arch} checksum differs between Makefile and Dockerfile")
    return problems


def cmd_verify() -> int:
    problems = atlas_mismatches(BACKEND_MAKEFILE.read_text(), DOCKERFILE.read_text())
    for p in problems:
        print(p, file=sys.stderr)
    return 1 if problems else 0


def cmd_check() -> int:
    cooldown = dt.date.today() - dt.timedelta(days=COOLDOWN_DAYS)
    outdated = 0
    for tool in TOOLS:
        current = pinned(tool)
        newest, released = latest(tool, cooldown)
        if newest == current:
            print(f"{tool}: {current} (current)")
            continue
        outdated += 1
        print(f"{tool}: {current} -> {newest} (released {released}): python3 scripts/tool_versions.py bump {tool}")
    return OUTDATED if outdated else 0


def cmd_bump(tool: str, version: str | None) -> int:
    cooldown = dt.date.today() - dt.timedelta(days=COOLDOWN_DAYS)
    released = None
    if version is None:
        version, released = latest(tool, cooldown)
    spec = TOOLS[tool]
    sums = new_checksums(tool, version)
    text = spec["file"].read_text()
    text = set_make_var(text, spec["version_var"], f"v{version}" if tool == "atlas" else version)
    for var, value in sums.items():
        text = set_make_var(text, var, value)
    if tool == "atlas":
        released = released or atlas_released(version)
        if released is None:
            raise SystemExit(f"Atlas v{version} is not on {ATLAS_CDN}")
        text = set_make_var(text, "ATLAS_RELEASED", released.isoformat())
        DOCKERFILE.write_text(rewrite_dockerfile_atlas(
            DOCKERFILE.read_text(), version,
            sums["ATLAS_COMMUNITY_SHA256_linux-amd64"], sums["ATLAS_COMMUNITY_SHA256_linux-arm64"],
        ))
    spec["file"].write_text(text)
    print(f"{tool} -> {version}; checksums from the release's published checksum files")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="cmd", required=True)
    sub.add_parser("verify")
    sub.add_parser("check")
    bump = sub.add_parser("bump")
    bump.add_argument("tool", choices=sorted(TOOLS))
    bump.add_argument("--version")
    args = parser.parse_args()
    if args.cmd == "verify":
        return cmd_verify()
    if args.cmd == "check":
        return cmd_check()
    return cmd_bump(args.tool, args.version)


if __name__ == "__main__":
    sys.exit(main())
