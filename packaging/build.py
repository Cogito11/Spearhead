#!/usr/bin/python3
"""Build and verify complete native Qt 6/QML Linux releases."""
import argparse
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tempfile
import urllib.request
from common import (
    ARTIFACTS,
    BUILD,
    DIST,
    EPOCH,
    REPORTS,
    ROOT,
    RUNTIME_HASH,
    RUNTIME_FILES,
    VERSION,
    run,
    sha256,
)


def qt_prefix():
    explicit = os.environ.get("SPEARHEAD_QT_PREFIX")
    if explicit:
        return Path(explicit)
    cached = BUILD / "deps/qt6/usr"
    return cached if cached.exists() else Path("/usr")


def qt_env():
    prefix = qt_prefix()
    lib = prefix / "lib/x86_64-linux-gnu"
    return dict(
        os.environ,
        LD_LIBRARY_PATH=str(lib),
        QT_PLUGIN_PATH=str(lib / "qt6/plugins"),
        QML_IMPORT_PATH=str(lib / "qt6/qml"),
        QT_QUICK_BACKEND="software",
        QT_QUICK_CONTROLS_STYLE="Basic",
        QT_STYLE_OVERRIDE="Fusion",
        QT_QPA_PLATFORMTHEME="generic",
    )


def logged(name, command, env=None, cwd=ROOT):
    print("Checking " + name + " …", flush=True)
    with (REPORTS / (name + ".log")).open("w") as output:
        result = subprocess.run(
            [str(x) for x in command],
            cwd=cwd,
            env=env,
            stdout=output,
            stderr=subprocess.STDOUT,
        )
    if result.returncode:
        print((REPORTS / (name + ".log")).read_text()[-14000:])
        raise SystemExit(name + " failed; see .build/reports/" + name + ".log")


def preflight(release):
    if platform.machine() != "x86_64":
        raise SystemExit("Build on Linux AMD64; see docs/BUILDING.md.")
    go = (
        os.environ.get("SPEARHEAD_GO")
        or shutil.which("go")
        or str(BUILD / "deps/go/bin/go")
    )
    required = [go, "cmake", "make", "g++"]
    if release:
        required += [
            "mksquashfs",
            "dpkg-deb",
            "ldconfig",
            "readelf",
            "desktop-file-validate",
            "wmctrl",
            "ip",
            "avahi-browse",
            "avahi-resolve-address",
        ]
    missing = [x for x in required if not shutil.which(x)]
    if missing:
        raise SystemExit(
            "Missing tools: " + ", ".join(missing) + ". See docs/BUILDING.md."
        )
    if not (qt_prefix() / "lib/x86_64-linux-gnu/cmake/Qt6").exists():
        raise SystemExit(
            "Qt 6 development dependencies are missing; see docs/BUILDING.md."
        )
    if release and not os.environ.get("DISPLAY"):
        raise SystemExit(
            "Package lifecycle checks need X11/XWayland or Xvfb with a tray. See docs/BUILDING.md."
        )
    return go


def native_build():
    env = qt_env()
    logged(
        "cmake",
        [
            "cmake",
            "-S",
            ROOT / "desktop",
            "-B",
            BUILD / "native",
            "-DCMAKE_PREFIX_PATH=" + str(qt_prefix()),
            "-DCMAKE_BUILD_TYPE=Release",
            "-DCMAKE_SKIP_RPATH=ON",
        ],
        env=env,
    )
    logged(
        "native-build",
        ["cmake", "--build", BUILD / "native", "-j" + str(min(os.cpu_count() or 2, 4))],
        env=env,
    )


def source_tests(go):
    logged("go-tests", [go, "test", "-race", "./..."])
    logged("go-vet", [go, "vet", "./..."])
    native_build()
    env = dict(
        qt_env(),
        QT_QPA_PLATFORM="offscreen",
        SPEARHEAD_TEST_OUTPUT=str(REPORTS / "native"),
    )
    logged("native-ui", [BUILD / "native/spearhead-desktop-test"], env=env)


def runtime():
    deps = BUILD / "deps"
    deps.mkdir(exist_ok=True)
    for name, url, expected in RUNTIME_FILES:
        target = deps / name
        if not target.exists():
            with urllib.request.urlopen(
                urllib.request.Request(url, headers={"User-Agent": "SpearHead-build"}),
                timeout=60,
            ) as response:
                data = response.read()
            temporary = target.with_suffix(".download")
            temporary.write_bytes(data)
            if sha256(temporary) != expected:
                temporary.unlink()
                raise SystemExit(
                    "Upstream runtime changed; review and update the pinned hashes."
                )
            temporary.replace(target)
        if sha256(target) != expected:
            raise SystemExit("Invalid cached runtime: " + str(target))
    (deps / "runtime-x86_64").chmod(0o755)
    return deps / "runtime-x86_64"


def verify_deb(package):
    with tempfile.TemporaryDirectory(prefix="spearhead-deb-check-") as temporary:
        path = Path(temporary)
        run(["dpkg-deb", "-x", package, path])
        run(
            [
                "desktop-file-validate",
                path / "usr/share/applications/io.spearhead.SpearHead.desktop",
            ]
        )
        assert (
            path / "usr/bin/spearhead"
        ).resolve() == path / "usr/lib/spearhead/spearhead"
        assert sha256(path / "usr/lib/spearhead/spearhead.png") == sha256(
            ROOT / "desktop/assets/brand.png"
        )
        assert not (path / "usr/lib/spearhead/spearhead-webview.py").exists()
        run(
            [
                path / "usr/lib/spearhead/spearhead",
                "--home",
                path / "profile",
                "init",
                "--name",
                "Package QA",
            ],
            stdout=subprocess.DEVNULL,
        )
        run(
            [path / "usr/lib/spearhead/spearhead-desktop", "--runtime-check"],
            env=dict(qt_env(), QT_QPA_PLATFORM="offscreen"),
        )
        (REPORTS / "deb.json").write_text(
            json.dumps(
                {
                    "version": VERSION,
                    "extraction": True,
                    "native_runtime": True,
                    "launcher": True,
                    "identity_initialization": True,
                },
                indent=2,
            )
            + "\n"
        )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--test",
        action="store_true",
        help="run source tests only, leaving dist untouched",
    )
    args = parser.parse_args()
    os.chdir(ROOT)
    BUILD.mkdir(exist_ok=True)
    REPORTS.mkdir(exist_ok=True)
    go = preflight(not args.test)
    source_tests(go)
    if args.test:
        print("All native source tests passed.")
        return
    if DIST.is_symlink():
        raise SystemExit("Refusing to clear a symlinked dist directory.")
    if DIST.exists():
        shutil.rmtree(DIST)
    DIST.mkdir()
    ARTIFACTS.mkdir(exist_ok=True)
    run(
        [
            go,
            "build",
            "-buildvcs=false",
            "-trimpath",
            "-ldflags=-s -w",
            "-o",
            ARTIFACTS / "spearhead",
            "./cmd/spearhead",
        ],
        env=dict(os.environ, CGO_ENABLED="0", GOOS="linux", GOARCH="amd64"),
    )
    shutil.copy2(BUILD / "native/spearhead-desktop", ARTIFACTS / "spearhead-desktop")
    shutil.copy2(BUILD / "native/spearhead-window", ARTIFACTS / "spearhead-window")
    shutil.copy2(ROOT / "desktop/assets/brand.png", ARTIFACTS / "spearhead.png")
    # Previous staging extras never belong to the native release.
    for obsolete in ("spearhead-webview.py", "spearhead.svg"):
        (ARTIFACTS / obsolete).unlink(missing_ok=True)
    env = dict(
        os.environ,
        SOURCE_DATE_EPOCH=str(EPOCH),
        SPEARHEAD_APPIMAGE_RUNTIME=str(runtime()),
        SPEARHEAD_QT_PREFIX=str(qt_prefix()),
    )
    logged(
        "appimage-build", ["/usr/bin/python3", ROOT / "packaging/appimage.py"], env=env
    )
    logged("deb-build", ["/usr/bin/python3", ROOT / "packaging/deb.py"], env=env)
    image = DIST / "spearhead-linux-amd64.AppImage"
    env = dict(
        os.environ,
        QT_QPA_PLATFORM="offscreen",
        SPEARHEAD_TEST_LIBRARY_PATH=str(qt_prefix() / "lib/x86_64-linux-gnu"),
        SPEARHEAD_TEST_OUTPUT=str(REPORTS / "native-appimage"),
    )
    logged(
        "packaged-native-ui",
        [image, "--runtime-test", BUILD / "native/spearhead-desktop-test"],
        env=env,
    )
    logged(
        "native-visual-parity",
        [
            "/usr/bin/python3",
            ROOT / "packaging/tests/visual_compare.py",
            REPORTS / "native",
            REPORTS / "native-appimage",
        ],
    )
    for report in REPORTS.glob("appimage-*.json"):
        report.unlink()
    logged(
        "appimage-lifecycle", ["/usr/bin/python3", ROOT / "packaging/tests/appimage.py"]
    )
    logged(
        "appimage-extract-lifecycle",
        ["/usr/bin/python3", ROOT / "packaging/tests/appimage.py"],
        env=dict(os.environ, APPIMAGE_EXTRACT_AND_RUN="1"),
    )
    verify_deb(DIST / f"spearhead_{VERSION}_amd64.deb")
    packages = sorted(p for p in DIST.iterdir() if p.suffix in (".AppImage", ".deb"))
    info = {
        "version": VERSION,
        "source_date_epoch": EPOCH,
        "target": "Linux AMD64; Ubuntu 24.04 / Mint 22; glibc >= 2.38; X11/XWayland",
        "ui": "Native Qt 6 / Qt Quick / QML; software scene graph",
        "go": subprocess.check_output([go, "version"], text=True).strip(),
        "appimage_runtime_sha256": RUNTIME_HASH,
        "validation": [
            "Go race tests",
            "Go vet",
            "Complete native UI/API regressions",
            "Native and packaged Qt screenshots",
            "60-frame native logo stress",
            "Read-once/session privacy",
            "Streaming file integrity",
            "AppImage FUSE/extract lifecycle",
            "Debian package extraction/native runtime",
        ],
        "visual_comparison": json.loads(
            (REPORTS / "native-appimage/comparison.json").read_text()
        ),
        "appimage_execution": {
            mode: json.loads((REPORTS / ("appimage-" + mode + ".json")).read_text())
            for mode in ("fuse", "extract")
            if (REPORTS / ("appimage-" + mode + ".json")).exists()
        },
        "files": {
            p.name: {"bytes": p.stat().st_size, "sha256": sha256(p)} for p in packages
        },
    }
    (DIST / "BUILD-INFO.json").write_text(json.dumps(info, indent=2) + "\n")
    (DIST / "SHA256SUMS").write_text(
        "".join(
            f"{sha256(p)}  {p.name}\n" for p in packages + [DIST / "BUILD-INFO.json"]
        )
    )
    shutil.rmtree(BUILD / "SpearHead.AppDir")
    (BUILD / "spearhead-appimage.squashfs").unlink()
    print(
        f"SpearHead {VERSION}: verified native QML AppImage and .deb in dist/.",
        flush=True,
    )


if __name__ == "__main__":
    main()
