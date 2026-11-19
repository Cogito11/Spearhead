#!/usr/bin/python3
"""Compare native and packaged QML geometry and raw rendered pixel hashes."""
from pathlib import Path
import json
import sys

reference = Path(sys.argv[1])
actual = Path(sys.argv[2])
a = json.loads((reference / "metrics.json").read_text())
b = json.loads((actual / "metrics.json").read_text())
differences = []


def compare(left, right, path):
    if isinstance(left, dict) and isinstance(right, dict):
        if left.keys() != right.keys():
            differences.append(path + " keys")
            return
        for key in left:
            compare(left[key], right[key], path + "." + key)
    elif isinstance(left, list) and isinstance(right, list):
        if len(left) != len(right):
            differences.append(path + " length")
            return
        for i, (x, y) in enumerate(zip(left, right)):
            compare(x, y, path + "." + str(i))
    elif isinstance(left, (int, float)) and isinstance(right, (int, float)):
        if abs(left - right) > 0.01:
            differences.append(path + f": {left} != {right}")
    elif left != right:
        differences.append(path)


compare(a, b, "QtQuick")
result = {
    "renderer": "Native Qt Quick/QML",
    "font": a["font"],
    "scenes": len(a["scenes"]),
    "differences": len(differences),
    "examples": differences[:20],
    "identical_geometry_and_pixels": not differences,
}
(actual / "comparison.json").write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps(result, indent=2))
assert not differences, "Packaged native rendering differs from the reference"
