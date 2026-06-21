#!/usr/bin/env python3
import os
import secrets
import string

def generate_secure_password(length=16):
    alphabet = string.ascii_letters + string.digits
    return "".join(secrets.choice(alphabet) for _ in range(length))

def generate_secret_key(length=48):
    return secrets.token_urlsafe(length)

def main():
    env_path = ".env"
    env_example_path = ".env-example"

    if os.path.exists(env_path):
        print(".env file already exists. Skipping initialization.")
        return

    if not os.path.exists(env_example_path):
        print(f"Error: {env_example_path} not found. Cannot initialize environment.")
        return

    print("Creating .env from .env-example...")
    with open(env_example_path, "r", encoding="utf-8") as f:
        content = f.read()

    secret_key = generate_secret_key()
    postgres_password = generate_secure_password()

    # Replace placeholders
    content = content.replace("POSTGRES_PASSWORD=changeme", f"POSTGRES_PASSWORD={postgres_password}")
    content = content.replace("postgres:changeme@", f"postgres:{postgres_password}@")
    content = content.replace("SECRET_KEY=change-me-to-a-random-string-at-least-32-chars", f"SECRET_KEY={secret_key}")

    with open(env_path, "w", encoding="utf-8") as f:
        f.write(content)

    print("Successfully initialized .env with secure random keys!")
    print(f"Generated SECRET_KEY: {secret_key[:10]}...")
    print("Generated POSTGRES_PASSWORD successfully.")

if __name__ == "__main__":
    main()
