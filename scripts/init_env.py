#!/usr/bin/env python3
"""Create .env from .env-example and the database password files compose mounts as secrets.

Existing files are never overwritten, so re-running cannot rotate a password the database volume already uses.
"""

import os
import secrets
import shutil

ENV_PATH = ".env"
ENV_EXAMPLE_PATH = ".env-example"
SECRETS_DIR = ".secrets"
# docker-compose.yml `secrets:`
SECRET_NAMES = ("postgres_password", "db_owner_password", "db_app_password")


def write_secret(path: str) -> bool:
    try:
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o444)
    except FileExistsError:
        if os.path.getsize(path) == 0:
            raise SystemExit(f"{path} is empty (an interrupted earlier run?); delete it and run again.")
        return False
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        f.write(secrets.token_urlsafe(32) + "\n")
    # 0444: file secrets are bind mounts, so compose's uid/mode options do not apply; the postgres and nonroot
    # users inside the containers must be able to read them. The 0700 directory keeps other host users out.
    os.chmod(path, 0o444)
    return True


def main() -> None:
    if os.path.exists(ENV_PATH):
        print(".env already exists; leaving it unchanged.")
    elif not os.path.exists(ENV_EXAMPLE_PATH):
        raise SystemExit(f"{ENV_EXAMPLE_PATH} not found; run from the repository root.")
    else:
        shutil.copyfile(ENV_EXAMPLE_PATH, ENV_PATH)
        print("Created .env from .env-example.")

    os.makedirs(SECRETS_DIR, mode=0o700, exist_ok=True)
    os.chmod(SECRETS_DIR, 0o700)
    for name in SECRET_NAMES:
        path = os.path.join(SECRETS_DIR, name)
        print(("Created " if write_secret(path) else "Kept existing ") + path)


if __name__ == "__main__":
    main()
