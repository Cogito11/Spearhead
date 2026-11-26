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
# Pin the runtime to an immutable, dated upstream release. The "continuous"
# release is overwritten on every upstream commit, so its bytes can never match
# a fixed hash for long and would break the build whenever upstream publishes.
RUNTIME_TAG = "20251108"
RUNTIME_COMMIT = "dd6cebedcbddde9c82f89b011e8e1d40b6e43868"
RUNTIME_HASH = "2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d"
RUNTIME_SOURCE = f"https://github.com/AppImage/type2-runtime/tree/{RUNTIME_TAG}"


def sha256(path):
    with Path(path).open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def run(command, **kwargs):
    return subprocess.run([str(item) for item in command], check=True, **kwargs)


RUNTIME_FILES = (
    (
        "runtime-x86_64",
        f"https://github.com/AppImage/type2-runtime/releases/download/{RUNTIME_TAG}/runtime-x86_64",
        RUNTIME_HASH,
    ),
    (
        "runtime-LICENSE",
        f"https://raw.githubusercontent.com/AppImage/type2-runtime/{RUNTIME_COMMIT}/LICENSE",
        "aa154fc9070614bbe7921f89db11efd1dba7a1f3a41685958110e2230f9c0ca1",
    ),
)
