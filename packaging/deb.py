#!/usr/bin/env python3
"""Build a Linux AMD64 installer without changing the host installation."""
from datetime import datetime, timezone
from pathlib import Path
import gzip
import hashlib
import os
import shutil
import subprocess
import tempfile

from common import (
    ROOT as root,
    DIST as dist,
    ARTIFACTS,
    DOCUMENTS,
    EPOCH as epoch,
    VERSION as version,
)

subprocess.run(["dpkg", "--validate-version", version], check=True)
output = dist / f"spearhead_{version}_amd64.deb"
with tempfile.TemporaryDirectory(prefix="spearhead-deb-") as temporary:
    stage = Path(temporary) / "package"
    app = stage / "usr/lib/spearhead"
    app.mkdir(parents=True)
    for source, name, mode in [
        (ARTIFACTS / "spearhead", "spearhead", 0o755),
        (ARTIFACTS / "spearhead-desktop", "spearhead-desktop", 0o755),
        (ARTIFACTS / "spearhead-window", "spearhead-window", 0o755),
        (root / "desktop/assets/brand.svg", "spearhead.svg", 0o644),
        (root / "desktop/assets/brand.png", "spearhead.png", 0o644),
    ]:
        shutil.copyfile(source, app / name)
        (app / name).chmod(mode)
    binary = stage / "usr/bin"
    binary.mkdir(parents=True)
    (binary / "spearhead").symlink_to("../lib/spearhead/spearhead")
    applications = stage / "usr/share/applications"
    applications.mkdir(parents=True)
    (applications / "io.spearhead.SpearHead.desktop").write_text(
        """[Desktop Entry]
Type=Application
Version=1.0
Name=SpearHead
Comment=Private LAN messages and file sharing
Exec=/usr/bin/spearhead app
Icon=spearhead
Terminal=false
Categories=Network;InstantMessaging;
StartupNotify=true
StartupWMClass=SpearHead
"""
    )
    icons = stage / "usr/share/icons/hicolor/scalable/apps"
    icons.mkdir(parents=True)
    shutil.copyfile(root / "desktop/assets/brand.svg", icons / "spearhead.svg")
    raster_icons = stage / "usr/share/icons/hicolor/512x512/apps"
    raster_icons.mkdir(parents=True)
    shutil.copyfile(root / "desktop/assets/brand.png", raster_icons / "spearhead.png")
    docs = stage / "usr/share/doc/spearhead"
    docs.mkdir(parents=True)
    for name in DOCUMENTS:
        target = docs / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(root / name, target)
    # Keep the IEEE attribution beside the embedded vendor database attribution.
    shutil.copyfile(root / "internal/lan/data/README.md", docs / "VENDOR-DATA.md")
    (docs / "changelog.gz").write_bytes(
        gzip.compress(
            f"""spearhead ({version}) stable; urgency=medium

  * Complete native Qt 6/QML desktop with matching styling and all messaging features.

 -- SpearHead contributors <spearhead@localhost>  {datetime.fromtimestamp(epoch,timezone.utc).strftime('%a, %d %b %Y %H:%M:%S +0000')}
""".encode(),
            mtime=epoch,
        )
    )
    size = sum(
        p.stat().st_size for p in stage.rglob("*") if p.is_file() and not p.is_symlink()
    )
    metadata = stage / "DEBIAN"
    metadata.mkdir()
    (metadata / "control").write_text(
        f"""Package: spearhead
Version: {version}
Section: net
Priority: optional
Architecture: amd64
Maintainer: SpearHead contributors <spearhead@localhost>
Installed-Size: {(size+1023)//1024}
Depends: libc6 (>= 2.38), libstdc++6, libgcc-s1, libqt6core6t64 (>= 6.4) | libqt6core6 (>= 6.4), libqt6gui6t64 (>= 6.4) | libqt6gui6 (>= 6.4), libqt6widgets6t64 (>= 6.4) | libqt6widgets6 (>= 6.4), libqt6network6t64 (>= 6.4) | libqt6network6 (>= 6.4), libqt6dbus6t64 (>= 6.4) | libqt6dbus6 (>= 6.4), libqt6qml6 (>= 6.4), libqt6quick6 (>= 6.4), libqt6quickcontrols2-6 (>= 6.4), libqt6svg6 (>= 6.4), qt6-qpa-plugins, qml6-module-qtquick, qml6-module-qtquick-controls, qml6-module-qtquick-layouts, qml6-module-qtquick-dialogs, qml6-module-qtquick-templates, qml6-module-qtquick-window, qml6-module-qtqml-workerscript, qml6-module-qtqml-models, qml6-module-qt-labs-folderlistmodel, fonts-noto-core, fonts-dejavu-core, iproute2
Recommends: avahi-utils
Description: Private LAN desktop messaging and file sharing
 SpearHead provides encrypted direct messages, a contact book, grouped network
 discovery, file sharing, and a persistent system tray. Its fully native Qt Quick/QML window is released when closed to the tray.
 User identities and history remain in the user's configuration directory.
"""
    )
    (metadata / "md5sums").write_text(
        "".join(
            f"{hashlib.md5(p.read_bytes()).hexdigest()}  {p.relative_to(stage)}\n"
            for p in sorted(stage.rglob("*"))
            if p.is_file() and not p.is_symlink() and metadata not in p.parents
        )
    )
    for path in stage.rglob("*"):
        if path.is_symlink():
            continue
        if path.is_dir():
            path.chmod(0o755)
        elif path.parent != app:
            path.chmod(0o644)
        os.utime(path, (epoch, epoch), follow_symlinks=False)
    subprocess.run(
        ["dpkg-deb", "--root-owner-group", "--build", str(stage), str(output)],
        env=dict(os.environ, SOURCE_DATE_EPOCH=str(epoch)),
        check=True,
    )
print(f"{output.name}: {output.stat().st_size:,} bytes")
