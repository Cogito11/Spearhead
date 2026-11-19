# Building the native desktop

## Environment

Use Linux AMD64, Ubuntu 24.04 / Mint 22, Go 1.22+, CMake 3.21+, a C++17 compiler,
Qt 6.4+, and Python 3.11+ for packaging only. Production has no Python dependency.

```sh
sudo apt update
sudo apt install golang-go build-essential cmake qt6-base-dev   qt6-declarative-dev qt6-svg-dev qt6-qpa-plugins   qml6-module-qtquick qml6-module-qtquick-controls   qml6-module-qtquick-layouts qml6-module-qtquick-dialogs   qml6-module-qtquick-templates qml6-module-qtquick-window   qml6-module-qtqml-workerscript qml6-module-qtqml-models   qml6-module-qt-labs-folderlistmodel fonts-noto-core fonts-dejavu-core   fontconfig iproute2 avahi-utils squashfs-tools desktop-file-utils   wmctrl xvfb xauth openbox stalonetray
```

`SPEARHEAD_GO` selects a custom Go executable; a cached toolchain at
`.build/deps/go/bin/go` is also recognized. `SPEARHEAD_QT_PREFIX` selects a Qt
installation prefix. A project-local extracted Qt runtime/toolchain at
`.build/deps/qt6/usr` is recognized automatically; otherwise `/usr` is used.
The current workspace uses that local cache, without a system installation.

`SOURCE_DATE_EPOCH` sets package timestamps. Repeatable commands do not promise
identical bytes across different compiler or distribution-library versions.
The AppImage runtime/license are downloaded once and checked against pinned
hashes; `.build/deps/` supports subsequent offline builds.

## Commands

```sh
./scripts/test.sh
./scripts/build.sh
```

Source tests use Qt's offscreen platform. Production package lifecycle tests
need an X11/XWayland display, window manager and tray. For headless builds:

```sh
xvfb-run -a -s '-screen 0 1280x900x24' sh -c 'openbox >/tmp/spearhead-openbox.log 2>&1 & stalonetray >/tmp/spearhead-tray.log 2>&1 & sleep 1; exec ./scripts/build.sh'
```

After source checks pass, the release build clears `dist/` and creates:

```text
dist/
  spearhead-linux-amd64.AppImage
  spearhead_<VERSION>_amd64.deb
  SHA256SUMS
  BUILD-INFO.json
```

Qt models, native controls, every migrated action, file integrity, read-once
privacy, pending-action races, RAM session restoration and repeated logo painting
are exercised through isolated local fixtures. Edge gestures at both ends of lists
and dialogs verify the subtle bounce and return, while contact-refresh and network
context-menu regressions verify detail navigation. Twenty-six light/dark scenes include
chat, picker, search, context menu, settings, contacts, network, narrow and scaled
views, including fractional text scales in Settings. The packaged runtime repeats the same tests. Qt library origins, exact
rendered-pixel hashes and geometry must match the native source reference.

Production AppImage checks initialize an isolated profile, verify native-only
libraries and icon data, start hidden, open/close/reopen, and quit. Both FUSE
(where available) and extract-and-run paths are checked, including complete window,
tray and service shutdown when Close to system tray is disabled; proportional memory
measurements are recorded. Debian payload checks never install it on the host.

Detailed results are in `.build/reports/`. Verify downloads with:

```sh
(cd dist && sha256sum -c SHA256SUMS)
```

Large AppDir/squashfs intermediates are removed after success. Compilers and Qt
packages in `.build/deps/` are build caches, not application content. The ignored
pre-QML source recovery archive is retained in `.build/recovery/` and never packaged.
