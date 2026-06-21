#!/usr/bin/env python3
import sys
import os

def replace_in_file(file_path, old_str, new_str):
    if not os.path.exists(file_path):
        print(f"Warning: File {file_path} not found, skipping.")
        return
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            content = f.read()
        new_content = content.replace(old_str, new_str)
        if new_content != content:
            with open(file_path, 'w', encoding='utf-8') as f:
                f.write(new_content)
            print(f"Updated {file_path}")
    except Exception as e:
        print(f"Error updating {file_path}: {e}")

def main():
    if len(sys.argv) < 2:
        print("Usage: python rename.py <new_name>")
        sys.exit(1)
    
    new_name = sys.argv[1]
    print(f"Renaming project to '{new_name}'...")

    # Replacements: (file, old, new)
    replacements = [
        ("docker-compose.yml", "fullstack-backend", f"{new_name}-backend"),
        ("docker-compose.dev.yml", "fullstack-backend", f"{new_name}-backend"),
        ("docker-compose.yml", "fullstack-frontend", f"{new_name}-frontend"),
        ("docker-compose.dev.yml", "fullstack-frontend", f"{new_name}-frontend"),
        ("frontend/package.json", "fullstack-template-frontend", f"{new_name}-frontend"),
        ("backend/pyproject.toml", "fastapi-template", f"{new_name}-backend"),
        (".env-example", "fullstack", new_name),
    ]

    for file_path, old, new in replacements:
        replace_in_file(file_path, old, new)

    # Multi-file replacements for branding
    brand_files = [
        "frontend/src/app/layout.tsx",
        "frontend/messages/en.json",
        "frontend/messages/zh-TW.json",
        "CLAUDE.md",
        "README.md"
    ]
    for file_path in brand_files:
        replace_in_file(file_path, "Fullstack Template", new_name)

    print("Done! Review changes with: git diff")
    print("Don't forget to update .env if it exists.")

if __name__ == "__main__":
    main()
