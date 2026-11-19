#!/bin/sh
set -eu
export PYTHONDONTWRITEBYTECODE=1
task_project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec /usr/bin/python3 "$task_project_root/packaging/build.py" "$@"
