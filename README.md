# SpearHead

Native Qt 6/Qt Quick/QML desktop messaging, contacts, LAN inventory and encrypted
file sharing. The Go service owns identity, networking and storage; the native
interface retains SpearHead's royal blue/silver styling and light/dark/system themes.
There is no browser interface or Python/GTK/WebKit runtime in the application.

## Run

```sh
chmod +x spearhead-linux-amd64.AppImage
./spearhead-linux-amd64.AppImage
```

The AppImage contains the complete native runtime and matching logo. The Debian
package installs a launcher and declares Qt 6 runtime dependencies. Release
baseline: Linux AMD64, Ubuntu 24.04 / Mint 22, glibc 2.38+, X11/XWayland.
Quit an older running version from its tray before launching an updated build.

Closing the window releases the QML window process while the service and tray
continue running. Reopening restores unsent drafts and contact edits from RAM.
Uncheck **Close to system tray** in Settings to quit completely when closing the
window. Theme, text size, alerts and the close preference persist between launches.

## Build

```sh
./scripts/build.sh     # source tests, fresh AppImage/.deb, package validation
./scripts/test.sh      # source checks without changing dist
```

`dist/` contains only the current AppImage, Debian installer, checksums and build
metadata. Build tools, intermediates and validation evidence stay in `.build/`.

- [Usage](docs/USAGE.md)
- [Build environment and verification](docs/BUILDING.md)
- [Architecture and codebase structure](docs/ARCHITECTURE.md)
- [Release changes](CHANGELOG.md)

Messages and attachments are encrypted in transit. Linux history and identity
files are protected by filesystem permissions, not encrypted at rest. The custom
transport has not received an independent security audit.
