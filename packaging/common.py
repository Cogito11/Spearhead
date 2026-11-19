"""Paths and metadata shared by the Linux release builders."""

from pathlib import Path
import hashlib
import os
import subprocess
import time

ROOT = Path(__file__).resolve().parent.parent
BUILD = ROOT / ".build"
ARTIFACTS = BUILD / "artifacts"
REPORTS = BUILD / "reports"
DIST = ROOT / "dist"
VERSION = (ROOT / "VERSION").read_text().strip()
EPOCH = int(os.environ.get("SOURCE_DATE_EPOCH", time.time()))
DOCUMENTS = (
    "README.md",
    "CHANGELOG.md",
    "docs/BUILDING.md",
    "docs/USAGE.md",
    "docs/ARCHITECTURE.md",
)
RUNTIME_HASH = "156f4bdbde9c52d01814600013e0a273f0118dc2de98975f3c8c63427ec79074"
RUNTIME_SOURCE = "https://github.com/AppImage/type2-runtime/tree/8f39b89"


def sha256(path):
    with Path(path).open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def run(command, **kwargs):
    return subprocess.run([str(item) for item in command], check=True, **kwargs)


RUNTIME_FILES = (
    (
        "runtime-x86_64",
        "https://github.com/AppImage/type2-runtime/releases/download/continuous/runtime-x86_64",
        RUNTIME_HASH,
    ),
    (
        "runtime-LICENSE",
        "https://raw.githubusercontent.com/AppImage/type2-runtime/8f39b89/LICENSE",
        "aa154fc9070614bbe7921f89db11efd1dba7a1f3a41685958110e2230f9c0ca1",
    ),
)
