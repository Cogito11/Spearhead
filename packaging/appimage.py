#!/usr/bin/python3
"""Bundle only the native Qt Quick desktop, Go service, and their dependencies."""
from pathlib import Path
import json
import io
import tarfile
import os
import re
import shutil
import subprocess
from urllib.parse import quote
from common import (
    ROOT,
    BUILD,
    DIST,
    ARTIFACTS,
    DOCUMENTS,
    VERSION,
    EPOCH,
    RUNTIME_HASH,
    RUNTIME_SOURCE,
    sha256,
)

qt = Path(os.environ.get("SPEARHEAD_QT_PREFIX", str(BUILD / "deps/qt6/usr")))
if not qt.exists():
    qt = Path("/usr")
qtlib = qt / "lib/x86_64-linux-gnu"
env = dict(
    os.environ, LD_LIBRARY_PATH=str(qtlib) + ":" + os.environ.get("LD_LIBRARY_PATH", "")
)
stage = BUILD / "SpearHead.AppDir"
if stage.exists():
    shutil.rmtree(stage)
stage.mkdir(parents=True)
source_files = set()
elf_files = set()


def copy(source, target):
    source = Path(source)
    destination = stage / target
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, destination)
    destination.chmod(source.stat().st_mode & 0o777)
    source_files.add(source.resolve())
    with source.open("rb") as data:
        if data.read(4) == b"\x7fELF":
            elf_files.add(source.resolve())
    return destination


def tree(source, target, ignore=None):
    for source_file in sorted(Path(source).rglob("*")):
        if source_file.is_file() and not (ignore and ignore(source_file)):
            copy(source_file, Path(target) / source_file.relative_to(source))


for name in ("spearhead", "spearhead-desktop", "spearhead-window"):
    copy(ARTIFACTS / name, "usr/lib/spearhead/" + name)
elf_files.discard((ARTIFACTS / "spearhead").resolve())
for name in ("brand.png", "brand.svg"):
    copy(
        ROOT / "desktop/assets" / name,
        "usr/lib/spearhead/" + name.replace("brand", "spearhead"),
    )
copy(ROOT / "desktop/assets/brand.png", "spearhead.png")
(stage / ".DirIcon").symlink_to("spearhead.png")
copy(
    ROOT / "desktop/assets/brand.png",
    "usr/share/icons/hicolor/512x512/apps/spearhead.png",
)
copy(
    ROOT / "desktop/assets/brand.svg",
    "usr/share/icons/hicolor/scalable/apps/spearhead.svg",
)
plugins = qtlib / "qt6/plugins"
for folder, names in {
    "platforms": ["libqxcb.so", "libqoffscreen.so"],
    "imageformats": ["libqsvg.so"],
    "iconengines": ["libqsvgicon.so"],
    "tls": ["libqopensslbackend.so"],
}.items():
    for name in names:
        path = plugins / folder / name
        if path.exists():
            copy(path, "usr/plugins/" + folder + "/" + name)
qml = qtlib / "qt6/qml"
# Keep Basic controls and their implementations; do not bundle unused styles,
# designers, profilers, Python, GTK, WebKit, or development tooling.
for module in ("QtQml", "QtQuick"):
    tree(
        qml / module,
        "usr/qml/" + module,
        lambda p: any(
            part in ("Fusion", "Material", "Imagine", "Universal", "designer")
            for part in p.parts
        )
        or p.suffix == ".qmltypes",
    )
for module in ("Qt/labs/folderlistmodel",):
    if (qml / module).exists():
        tree(qml / module, "usr/qml/" + module, lambda p: p.suffix == ".qmltypes")
for executable in ("ip", "avahi-browse", "avahi-resolve-address"):
    source = shutil.which(executable)
    if not source:
        raise RuntimeError("Missing build dependency: " + executable)
    copy(source, "usr/bin/" + executable)
exclude = re.compile(
    r"^(ld-linux.*|libc\.so.*|libm\.so.*|libpthread\.so.*|libdl\.so.*|librt\.so.*|libresolv\.so.*|libnss_(files|dns|compat|hesiod)\.so.*)$"
)
scanned = set()
while elf_files - scanned:
    for source in sorted(elf_files - scanned):
        scanned.add(source)
        result = subprocess.check_output(
            ["ldd", str(source)], env=env, text=True, stderr=subprocess.STDOUT
        )
        if "not found" in result:
            raise RuntimeError(result)
        for line in result.splitlines():
            match = re.match(r"\s*(\S+) => (/.+?) \(", line)
            if (
                match
                and not exclude.match(match[1])
                and not (stage / "usr/lib" / match[1]).exists()
            ):
                copy(match[2], "usr/lib/" + match[1])
for style in ("Regular", "Bold", "Italic", "BoldItalic"):
    copy(
        "/usr/share/fonts/truetype/noto/NotoSans-" + style + ".ttf",
        "usr/share/fonts/truetype/noto/NotoSans-" + style + ".ttf",
    )
for name in ("DejaVuSansMono.ttf", "DejaVuSans.ttf"):
    copy(
        "/usr/share/fonts/truetype/dejavu/" + name,
        "usr/share/fonts/truetype/dejavu/" + name,
    )
font_rules = (
    Path("/etc/fonts/fonts.conf")
    .read_text()
    .replace(
        "<dir>/usr/local/share/fonts</dir>",
        '<dir>/usr/local/share/fonts</dir>\n<dir prefix="relative">fonts/truetype/noto</dir>\n<dir prefix="relative">fonts/truetype/dejavu</dir>',
    )
)
(stage / "usr/share/fonts.conf").write_text(font_rules)
source_files.add(Path("/etc/fonts/fonts.conf"))
tree("/etc/fonts/conf.d", "usr/share/conf.d")
copy("/usr/share/xml/fontconfig/fonts.dtd", "usr/share/fonts.dtd")
copy(ROOT / "packaging/appimage/AppRun", "AppRun")
(stage / "usr/bin/spearhead").symlink_to("../lib/spearhead/spearhead")
(stage / "usr/lib/spearhead/qt.conf").write_text(
    "[Paths]\nPlugins=../../plugins\nQmlImports=../../qml\n"
)
(stage / "spearhead.desktop").write_text(
    "[Desktop Entry]\nType=Application\nName=SpearHead\nComment=Private LAN messages and file sharing\nExec=spearhead\nIcon=spearhead\nCategories=Network;InstantMessaging;\nTerminal=false\nStartupWMClass=SpearHead\n"
)
for name in DOCUMENTS:
    copy(ROOT / name, "usr/share/doc/spearhead/" + name)
copy(ROOT / "internal/lan/data/README.md", "usr/share/doc/spearhead/VENDOR-DATA.md")
licenses = stage / "usr/share/doc/spearhead/third-party"
licenses.mkdir(parents=True)
packages = {}
system_paths = sorted(
    str(Path("/usr") / str(p)[1:]) if str(p).startswith("/lib/") else str(p)
    for p in source_files
    if str(p).startswith(("/usr/", "/lib/", "/etc/"))
)
for start in range(0, len(system_paths), 100):
    result = subprocess.run(
        ["dpkg-query", "-S", *system_paths[start : start + 100]],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    )
    for line in result.stdout.splitlines():
        if ": " in line:
            for package in line.rsplit(": ", 1)[0].split(", "):
                packages[package] = {}
for package in sorted(packages):
    fields = subprocess.check_output(
        [
            "dpkg-query",
            "-W",
            "-f=${Version}\t${source:Package}\t${source:Version}",
            package,
        ],
        text=True,
    ).split("\t")
    packages[package] = {
        "version": fields[0],
        "source_package": fields[1],
        "source_version": fields[2],
        "source_url": "https://launchpad.net/ubuntu/+source/"
        + quote(fields[1], safe="")
        + "/"
        + quote(fields[2], safe=""),
    }
    notice = Path("/usr/share/doc") / package.split(":")[0] / "copyright"
    if notice.exists():
        shutil.copyfile(notice, licenses / (package.replace(":", "_") + ".copyright"))
# Cached distribution packages remain accompanied by their own license/source metadata.
manifest = qt.parent / "packages.json"
if manifest.exists():
    for entry in json.loads(manifest.read_text()):
        fields = dict(
            line.split(": ", 1)
            for line in entry["metadata"].splitlines()
            if ": " in line
        )
        package = fields["Package"]
        version = fields["Version"]
        source = fields.get("Source", package)
        match = re.match(r"([^ ]+)(?: \((.+)\))?", source)
        source_name = match[1]
        source_version = match[2] or version
        # Only packages that actually contributed copied runtime files need notices.
        archive = BUILD / "deps/qt6-archives" / entry["file"]
        with tarfile.open(
            fileobj=io.BytesIO(
                subprocess.check_output(["dpkg-deb", "--fsys-tarfile", str(archive)])
            )
        ) as content:
            owned = {
                member.name.lstrip("./")
                for member in content.getmembers()
                if member.isfile()
            }
        contributed = {
            str(path.relative_to(qt.parent))
            for path in source_files
            if qt in path.parents
        }
        if not owned.intersection(contributed):
            continue
        packages[package] = {
            "version": version,
            "source_package": source_name,
            "source_version": source_version,
            "source_url": "https://launchpad.net/ubuntu/+source/"
            + quote(source_name, safe="")
            + "/"
            + quote(source_version, safe=""),
            "archive_sha256": entry["sha256"],
        }
        notice = qt / "share/doc" / package / "copyright"
        if notice.exists():
            shutil.copyfile(notice, licenses / (package + ".copyright"))
runtime = Path(os.environ["SPEARHEAD_APPIMAGE_RUNTIME"])
assert sha256(runtime) == RUNTIME_HASH
shutil.copyfile(
    runtime.with_name("runtime-LICENSE"), licenses / "AppImage-runtime.LICENSE"
)
info = {
    "version": VERSION,
    "architecture": "x86_64",
    "ui": "Native Qt 6 / Qt Quick / QML; software scene graph",
    "baseline": "Ubuntu 24.04 / Mint 22; glibc >= 2.38; X11/XWayland",
    "host_components": [
        "glibc",
        "display server",
        "graphics drivers",
        "desktop D-Bus and tray",
    ],
    "runtime_sha256": RUNTIME_HASH,
    "runtime_source": RUNTIME_SOURCE,
    "packages": packages,
}
(stage / "usr/share/doc/spearhead/BUNDLE-INFO.json").write_text(
    json.dumps(info, indent=2) + "\n"
)
versions = []
for path in (stage / "usr/lib").glob("*.so*"):
    output = subprocess.run(
        ["readelf", "--version-info", str(path)],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    ).stdout
    versions.extend(re.findall(r"Name: GLIBC_([0-9.]+)", output))
required = max(versions, key=lambda v: tuple(map(int, v.split("."))))
assert tuple(map(int, required.split("."))) <= (2, 38), required
subprocess.run(["desktop-file-validate", str(stage / "spearhead.desktop")], check=True)
subprocess.run([str(stage / "AppRun"), "--runtime-check"], check=True)
filesystem = BUILD / "spearhead-appimage.squashfs"
filesystem.unlink(missing_ok=True)
subprocess.run(
    [
        "mksquashfs",
        str(stage),
        str(filesystem),
        "-noappend",
        "-all-root",
        "-comp",
        "zstd",
        "-Xcompression-level",
        "15",
        "-processors",
        "2",
        "-no-progress",
        "-mkfs-time",
        str(EPOCH),
        "-all-time",
        str(EPOCH),
    ],
    check=True,
    env={k: v for k, v in os.environ.items() if k != "SOURCE_DATE_EPOCH"},
)
temporary = BUILD / "spearhead-appimage.tmp"
with temporary.open("wb") as output:
    output.write(runtime.read_bytes())
    with filesystem.open("rb") as source:
        shutil.copyfileobj(source, output)
temporary.chmod(0o755)
os.replace(temporary, DIST / "spearhead-linux-amd64.AppImage")
print(
    "Native AppImage:",
    (DIST / "spearhead-linux-amd64.AppImage").stat().st_size / 1048576,
    "MiB; minimum glibc",
    required,
)
