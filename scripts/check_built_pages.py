#!/usr/bin/env python3
"""Check pages of the frontend's production build: each answers its status and every script carries the CSP nonce.

Usage: python3 scripts/check_built_pages.py   (run from the repository root after `pnpm build` in frontend/)

A script without the response's nonce never runs, so the page never hydrates. That only shows on a production build:
`next dev` drops the policy (src/proxy.ts) and vitest renders no pages, so CI runs this after `pnpm build`. Case:
Next.js answers an unknown path with its built-in 404, prerendered at build time with no nonce on any script, unless
a route calls notFound() per request (src/app/[locale]/[...rest]/page.tsx).
"""

import os
import re
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request

FRONTEND = "frontend"
# path → expected status
PAGES = {
    "/en": 200,
    "/en/no-such-page": 404,
    "/zh-TW/a/b/c": 404,
}


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def fetch(url: str) -> tuple[int, str, str]:
    try:
        with urllib.request.urlopen(url, timeout=10) as r:
            return r.status, r.headers.get("content-security-policy", ""), r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.headers.get("content-security-policy", ""), e.read().decode()


def check(base: str) -> list[str]:
    problems = []
    for path, want in PAGES.items():
        status, csp, html = fetch(base + path)
        nonce = re.search(r"'nonce-([^']+)'", csp)
        tags = re.findall(r"<script\b[^>]*>", html)
        signed = [t for t in tags if nonce and f'nonce="{nonce[1]}"' in t]
        line = f"{path}: status {status}, {len(signed)} of {len(tags)} scripts carry the nonce"
        print(line)
        if status != want or not nonce or not tags or len(signed) != len(tags):
            problems.append(f"{line} (want status {want}, every script with the nonce)")
    return problems


def main() -> int:
    port = free_port()
    base = f"http://127.0.0.1:{port}"
    env = {
        **os.environ,
        # pages that call the API fail closed; the checked pages render without it
        "API_URL": os.environ.get("API_URL", "http://127.0.0.1:9"),
        "NEXT_PUBLIC_APP_URL": os.environ.get("NEXT_PUBLIC_APP_URL", base),
    }
    server = subprocess.Popen(
        # next itself, not through pnpm, so terminate() reaches the server and leaves no process holding the port
        ["node_modules/.bin/next", "start", "-H", "127.0.0.1", "-p", str(port)],
        cwd=FRONTEND,
        env=env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    try:
        deadline = time.monotonic() + 60
        while True:
            try:
                fetch(base + "/en")
                break
            # URLError, a refused connection and a read timeout during the cold start are all OSError
            except OSError:
                if server.poll() is not None or time.monotonic() > deadline:
                    print("next start did not answer within 60 s", file=sys.stderr)
                    return 1
                time.sleep(0.5)
        problems = check(base)
    finally:
        server.terminate()
        try:
            server.wait(timeout=10)
        except subprocess.TimeoutExpired:
            server.kill()
            server.wait()
    for p in problems:
        print(f"FAIL {p}", file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
