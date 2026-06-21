#!/usr/bin/env python3
import os
import socket
import sys
import time
from urllib.parse import urlparse


def main():
    database_url = os.getenv("DATABASE_URL")
    if not database_url:
        print("DATABASE_URL not set, skipping wait.")
        sys.exit(0)

    try:
        # urlparse handles postgresql:// and postgresql+asyncpg:// nicely
        parsed = urlparse(database_url)
        host = parsed.hostname or "localhost"
        port = parsed.port or 5432
    except Exception as e:
        print(f"Error parsing DATABASE_URL: {e}, skipping wait.")
        sys.exit(0)

    print(f"Waiting for database at {host}:{port}...")

    start_time = time.time()
    timeout = 60  # wait up to 60 seconds
    while time.time() - start_time < timeout:
        try:
            with socket.create_connection((host, port), timeout=2):
                print("Database is up and accepting connections!")
                sys.exit(0)
        except (TimeoutError, OSError):
            time.sleep(1)

    print(f"Error: Database at {host}:{port} did not become ready within {timeout} seconds.")
    sys.exit(1)

if __name__ == "__main__":
    main()
