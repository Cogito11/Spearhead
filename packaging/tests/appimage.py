#!/usr/bin/python3
"""Verify the production native AppImage without using installed user profiles."""
from configparser import ConfigParser
from pathlib import Path
import hashlib
import json
import os
import socket
import subprocess
import tempfile
import time
import urllib.request

project = Path(__file__).resolve().parents[2]
image = Path(
    os.environ.get(
        "SPEARHEAD_TEST_IMAGE", str(project / "dist/spearhead-linux-amd64.AppImage")
    )
)


def port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def info(pid):
    try:
        fields = (
            (Path("/proc") / str(pid) / "stat").read_text().split(") ", 1)[1].split()
        )
        return {"pid": pid, "parent": int(fields[1]), "state": fields[0]}
    except (FileNotFoundError, ProcessLookupError):
        return None


def family(pid):
    processes = {
        int(p.name): info(int(p.name))
        for p in Path("/proc").iterdir()
        if p.name.isdigit()
    }
    found = {pid}
    while True:
        children = {p for p, d in processes.items() if d and d["parent"] in found}
        if children <= found:
            return [
                p for p in found if processes.get(p) and processes[p]["state"] != "Z"
            ]
        found |= children


def wait(predicate, seconds=15):
    until = time.monotonic() + seconds
    while time.monotonic() < until:
        value = predicate()
        if value:
            return value
        time.sleep(0.1)
    raise AssertionError("Native AppImage lifecycle timed out")


def args(pid):
    try:
        return (Path("/proc") / str(pid) / "cmdline").read_bytes()
    except FileNotFoundError:
        return b""


def maps(pid):
    try:
        return (Path("/proc") / str(pid) / "maps").read_text()
    except FileNotFoundError:
        return ""


def pss(processes):
    total = 0
    for pid in processes:
        try:
            for line in (
                (Path("/proc") / str(pid) / "smaps_rollup").read_text().splitlines()
            ):
                if line.startswith("Pss:"):
                    total += int(line.split()[1])
        except (FileNotFoundError, PermissionError, ProcessLookupError):
            pass
    return total


with tempfile.TemporaryDirectory(prefix="spearhead-native-appimage-qa-") as directory:
    directory = Path(directory)
    home = directory / "profile"
    env = dict(os.environ, XDG_CONFIG_HOME=str(directory / "config"))
    if not Path("/dev/fuse").exists():
        env["APPIMAGE_EXTRACT_AND_RUN"] = "1"
    subprocess.run([str(image), "--runtime-check"], env=env, check=True)
    check = """from pathlib import Path
import os,json,hashlib,struct
root=Path(os.environ['SPEARHEAD_APPDIR']);icon=root/'.DirIcon';assert icon.resolve()==root/'spearhead.png'
data=icon.read_bytes();assert data[:8]==b'\\x89PNG\\r\\n\\x1a\\n';assert struct.unpack('>II',data[16:24])==(512,512)
assert (root/'usr/share/icons/hicolor/512x512/apps/spearhead.png').read_bytes()==data
assert not (root/'usr/lib/spearhead/spearhead-webview.py').exists()
assert not (root/'usr/bin/python3').exists()
assert not list((root/'usr/lib').glob('*webkit*'))
print(json.dumps({'size':512,'sha256':hashlib.sha256(data).hexdigest()}))
"""
    icon = json.loads(
        subprocess.check_output(
            [str(image), "--runtime-test", "/usr/bin/python3", "-c", check],
            env=env,
            text=True,
        )
    )
    assert (
        icon["sha256"]
        == hashlib.sha256(
            (project / "desktop/assets/brand.png").read_bytes()
        ).hexdigest()
    )
    subprocess.run(
        [str(image), "--home", str(home), "init", "--name", "Native AppImage QA"],
        env=env,
        stdout=subprocess.DEVNULL,
        check=True,
    )
    config = json.loads((home / "config.json").read_text())
    config.update(discovery=False, listen_port=port(), ui_port=port())
    (home / "config.json").write_text(json.dumps(config))
    with (directory / "output.log").open("w+") as log:
        backend = subprocess.Popen(
            [str(image), "--home", str(home), "app", "--background"],
            env=env,
            stdout=log,
            stderr=log,
        )
        try:
            wait(lambda: (home / "ui.json").exists())
            lock = json.loads((home / "ui.json").read_text())
            wait(
                lambda: any(
                    b"spearhead-desktop" in args(pid) for pid in family(backend.pid)
                )
            )
            time.sleep(0.5)
            assert not any(
                b"--window" in args(pid) for pid in family(backend.pid)
            ), "Background tray created a window"
            closed_before = pss(family(backend.pid))
            subprocess.run(
                [str(image), "--home", str(home), "app"],
                env=env,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=True,
                timeout=15,
            )
            renderer = wait(
                lambda: next(
                    (pid for pid in family(backend.pid) if b"--window" in args(pid)),
                    None,
                )
            )

            def window():
                for row in subprocess.check_output(
                    ["wmctrl", "-lp"], text=True
                ).splitlines():
                    fields = row.split(None, 4)
                    if len(fields) > 2 and fields[2] == str(renderer):
                        return fields[0]

            identifier = wait(window)
            time.sleep(0.6)
            opened = family(backend.pid)
            opened_pss = pss(opened)
            libraries = {}
            for pid in opened:
                assert lock["token"].encode() not in args(
                    pid
                ), "Private token appeared in process arguments"
                assert "libwebkit" not in maps(pid) and "libgtk-3" not in maps(
                    pid
                ), "Old UI runtime loaded"
                for line in maps(pid).splitlines():
                    if any(
                        name in line
                        for name in (
                            "libQt6Core",
                            "libQt6Gui",
                            "libQt6Widgets",
                            "libQt6Qml",
                            "libQt6Quick",
                            "libQt6Network",
                            "libQt6Svg",
                        )
                    ):
                        path = line.split(maxsplit=5)[-1]
                        assert "/.mount_" in path or "/appimage_extracted_" in path, (
                            "System UI library used: " + path
                        )
                        libraries[Path(path).name] = path
            for name in ("libQt6Qml", "libQt6Quick", "libQt6Widgets", "libQt6Svg"):
                assert any(name in path for path in libraries.values()), (
                    "Missing native UI library: " + name
                )
            subprocess.run(["wmctrl", "-ic", identifier], check=True)
            wait(
                lambda: not any(b"--window" in args(pid) for pid in family(backend.pid))
            )
            time.sleep(0.4)
            closed_after = pss(family(backend.pid))
            subprocess.run(
                [str(image), "--home", str(home), "app"],
                env=env,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=True,
                timeout=15,
            )
            wait(lambda: any(b"--window" in args(pid) for pid in family(backend.pid)))
            request = urllib.request.Request(
                f'http://127.0.0.1:{lock["port"]}/api/desktop/quit',
                data=b"{}",
                headers={
                    "Cookie": "spearhead=" + lock["token"],
                    "X-SpearHead": "1",
                    "Content-Type": "application/json",
                },
            )
            with urllib.request.urlopen(request, timeout=8) as response:
                assert response.status == 200
            backend.wait(timeout=10)
            for pid in opened:
                wait(lambda pid=pid: not info(pid) or info(pid)["state"] == "Z")
            # The Qt host must honor the persisted opt-out and stop the whole
            # application when its real window is closed, with no tray left behind.
            preference_file = directory / "config/SpearHead/SpearHead.conf"
            preferences = ConfigParser()
            preferences.optionxform = str
            preferences.read(preference_file)
            assert preferences.getboolean("ui", "closeToTray", fallback=True)
            if not preferences.has_section("ui"):
                preferences.add_section("ui")
            preferences.set("ui", "closeToTray", "false")
            with preference_file.open("w") as output:
                preferences.write(output, space_around_delimiters=False)
            backend = subprocess.Popen(
                [str(image), "--home", str(home), "app"],
                env=env, stdout=log, stderr=log,
            )
            renderer = wait(
                lambda: next(
                    (pid for pid in family(backend.pid) if b"--window" in args(pid)), None
                )
            )
            identifier = wait(window)
            time.sleep(0.5)
            closing = family(backend.pid)
            subprocess.run(["wmctrl", "-ic", identifier], check=True)
            backend.wait(timeout=10)
            assert backend.returncode == 0
            for pid in closing:
                wait(lambda pid=pid: not info(pid) or info(pid)["state"] == "Z")
            wait(lambda: not (home / "ui.json").exists())
            preferences.read(preference_file)
            assert not preferences.getboolean("ui", "closeToTray")
            mode = "extract" if "APPIMAGE_EXTRACT_AND_RUN" in env else "fuse"
            result = {
                "ui": "Native Qt 6 / Qt Quick / QML",
                "native_libraries_bundled": len(libraries),
                "embedded_appimage_icon": icon,
                "no_webkit_gtk_python_runtime": True,
                "private_url_in_argv": False,
                "closed_renderer_released": True,
                "reopened": True,
                "clean_shutdown": True,
                "close_to_tray_default": True,
                "close_to_tray_opt_out_full_shutdown": True,
                "close_preference_persisted": True,
                "runtime_mode": "extract-and-run" if mode == "extract" else "FUSE",
                "process_family_pss_kib": {
                    "background_before": closed_before,
                    "window_open": opened_pss,
                    "background_after": closed_after,
                },
            }
            (project / ".build/reports" / ("appimage-" + mode + ".json")).write_text(
                json.dumps(result, indent=2) + "\n"
            )
            print(json.dumps(result, indent=2))
        except Exception:
            log.flush()
            log.seek(0)
            diagnostic = log.read().replace(
                locals().get("lock", {}).get("token", "TOKEN-NOT-SET"), "[redacted]"
            )
            (project / ".build/reports/native-lifecycle-error.log").write_text(
                diagnostic
            )
            raise
        finally:
            if backend.poll() is None:
                backend.terminate()
                backend.wait(timeout=10)
