# Native application structure

```text
Spearhead/
├── cmd/spearhead/           CLI, automatic desktop setup and service lifecycle
├── internal/
│   ├── api/                Authenticated loopback JSON/file API and integration tests
│   ├── config/             Profile paths, defaults and legacy compatibility
│   ├── identity/           Keys, fingerprints and platform key protection
│   ├── discovery/          Signed peer announcements
│   ├── lan/                Bounded scans, device identification and deduplication
│   │   └── data/           Compressed IEEE vendor database and attribution
│   ├── node/               Contacts, retries, messages and encrypted file transfers
│   ├── protocol/           Signed records and wire compatibility
│   ├── transport/          Authenticated encrypted peer sessions
│   ├── store/              Atomic persistence, attachment storage, deletion receipts
│   ├── safety/             Sensitive-content detection
│   └── desktop/            Native tray-host launching
├── desktop/
│   ├── CMakeLists.txt      Qt 6 build, embedded resources and tests
│   ├── tray.cpp            Lightweight single-instance tray host
│   ├── window.cpp          Disposable native QML window process
│   ├── api_client.cpp/.h   Shared authenticated API client
│   ├── bridge.cpp/.h       API client, keyed models, navigation and UI state
│   ├── nativeicon.cpp/.h   CPU-painted vector icons and asymmetric chat bubbles
│   ├── style.h             Explicit fonts and native dialog palettes
│   ├── qml/                Main window, screens, dialogs and reusable styled controls
│   ├── assets/             Canonical branding and vector icon definitions
│   ├── desktop_test.cpp    Native feature, privacy, race and rendering tests
│   └── tests/              Isolated HTTP fixture and test-only data
├── packaging/              Native AppImage/.deb builders and release checks
│   ├── appimage/AppRun     Relocatable Qt runtime launcher
│   └── tests/              Lifecycle, memory and screenshot-parity verification
├── scripts/                build.sh and test.sh
├── docs/                   Usage, building and architecture
├── .build/                 Ignored tools, build products, recovery archive and QA reports
└── dist/                   Current release packages, checksums and metadata
```

## Process boundaries

The Go service owns peer networking, LAN discovery, identity, storage and the
outbox. It serves an authenticated JSON/file API on numeric loopback. Cookies,
Host restrictions, an application header and JSON-only action requests remain
in place. No HTML, CSS or browser JavaScript is served.

A Qt tray host owns singleton IPC, alerts and RAM-only UI session restoration.
It starts a separate QML executable, passing the private API URL
and session over stdin. Arguments never contain the private URL. The QML window
process owns its models and controls and exits on close-to-tray. All rendering
uses Qt Quick's software scene graph, with native vector painting for the logo.
This application does not load a web engine.

Closing captures drafts, current view, search, contact edits and appearance into
the tray host's RAM. Message bodies and revealed read-once content are excluded.
Only appearance, alerts, the close-to-tray preference and window dimensions are
written to Qt settings. Close-to-tray defaults to true; disabling it makes window
close shut down the renderer, tray host and Go service. Quit always ends all three. Close is deferred while a
send, save or file transfer is in flight.

## Native UI and correctness

The API bridge owns keyed message/sidebar models so polling can update individual
rows without rebuilding the whole view. Generation counters reject stale thread
and network responses. Sending captures recipient and draft state before any
secret confirmation. Contact-save revisions protect newer edits and other forms.
Read-once bodies exist only in the active window and disappear on navigation or
process exit. Clipboard actions use the native clipboard; message text is escaped
before safe HTTP(S) link rendering. Files stream through Qt network devices and
save atomically only after their length and SHA-256 match the message metadata.

Shared QML components preserve the existing palette, explicit Noto Sans typography,
spacing, selection states, menus and text scaling. Native file dialogs receive the
chosen palette. Screens remain responsive down to a narrow single-pane window.
Scroll-edge feedback is shared across lists and scroll views: native scroll bounds
stay fixed while a resisted visual displacement is capped at sixteen logical pixels.
Mouse-wheel and touchpad events receive a small edge pulse without consuming the
event; short content also gets feedback. The effect settles back smoothly.
Background contact updates use their own change signal so they cannot reset
narrow-window navigation. Explicit detail selections open the appropriate pane.

## Preserved backend behavior

Discovery and inventory retain signed peer identity, MAC/UUID merging, grouped
self interfaces, proxy-ARP guards, bounded physical-subnet probes, mDNS/SSDP,
reverse DNS, service hints and offline vendor information.

Transport remains ephemeral X25519, transcript-bound Ed25519, HKDF-SHA256 and
AES-256-GCM. Existing wire identifiers and profile migrations remain compatible.
Atomic JSON history is not encrypted at rest on Linux. Local deletion removes
bodies/attachments and retries while retaining body-free replay receipts.
Retries expire after seven days; default history retention is ninety days.

## Release and recovery

The AppImage bundles Go, Qt 6, required QML modules, fonts, launcher icons and
runtime dependencies with distribution license/source notices. The `.deb`
declares Qt runtime dependencies. Neither contains Python, GTK or WebKit.

The build runs Go race/vet checks, native API/UI tests, read-once/session privacy,
streamed file integrity, pending-action races, sixty logo repaint checks, twenty-six
native-versus-packaged screenshot comparisons, edge-gesture and narrow-navigation
regressions, and production tray lifecycle checks in FUSE/extract modes. Packaged
checks exercise both default background close and full shutdown after opting out. Logs, images and memory measurements stay under
`.build/reports/`; release evidence is summarized in `dist/BUILD-INFO.json`.

A single compressed pre-transition source snapshot is retained at
`.build/recovery/pre-qml-source.tar.gz`. It is excluded from the active source
graph, all builds and all release packages. Unpacked retired source, web UI
references, old Qt build trees and editor backups have been removed. Active
Qt 6 build dependencies and current native QA reports remain under `.build/`.
