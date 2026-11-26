# Changes

## Unreleased

- Move message search into the conversation top bar: the search field opens to
  the left of the search button, which becomes a close (×) button while open.
- Outline the search/close button to match the search field.
- Use an up arrow for the send button.
- Fix the release build by pinning the AppImage runtime to the immutable
  `20251108` upstream release instead of the overwritten `continuous` build.

## 0.3.4

- Toggle conversation search with its toolbar button, and close the sidebar menu
  when its selector is clicked again. Anchor keyboard-opened menus correctly.
- Add a persistent Close to system tray preference, enabled by default. Disable
  it to fully shut down the window, tray and service when the window closes.
- Preserve the chat scroll position when opening an empty search and avoid
  redundant search model updates, sidebar search work and avatar calculations.
- Remove unused transfer-reply tracking, temporary sidebar data and obsolete
  renderer-constructor/test scaffolding.

## 0.3.3

- Make edge bounce slightly stronger (16 logical pixels maximum) and restore
  feedback for mouse wheels, touchpads and content shorter than the viewport.
- Inset settings content and use equal-width layout allocation for theme buttons
  so the Dark button border stays visible at fractional text scales.
- Verify wheel feedback, normal scrolling, short chat content and both gesture edges.

## 0.3.2

- Soften scroll-edge feedback throughout lists, details, editors and dialogs with
  resisted movement capped at 10 logical pixels and a short smooth return.
- Keep narrow-window details open across background updates and correctly show
  contact details opened from context menus or restored sessions.
- Show device details when using the network list context menu, including generic
  LAN devices that have no associated SpearHead contact.

## 0.3.1

- Enlarge the sidebar logo and place nearby status beneath the SpearHead title.
- Remove the private/local tagline and composer keyboard hint; preserve keyboard shortcuts.
- Replace the attachment glyph with a clean paperclip across chat and file messages.
- Remove QML editor backups, the unused terminal setup wizard, and obsolete migration caches.
  Keep one compressed pre-transition recovery snapshot outside the active source tree.

## 0.3.0

- Complete native Qt 6/Qt Quick/QML desktop transition, including every screen,
  dialog, menu, search, contact action, read-once flow, setting and file transfer.
- Preserve the blue/silver design, explicit fonts, light/dark/system themes and
  responsive layouts with software rendering and CPU-painted vector branding.
- Keep the Go backend and existing identities/history/protocol compatible.
  Replace the browser UI with an authenticated native JSON/file API.
- Release the QML process on close-to-tray and restore drafts/contact edits from
  RAM; protect in-flight sends/saves/transfers and sensitive-message sessions.
- Bundle only Qt and necessary native dependencies in AppImage/.deb releases.
  Preserve the prior implementation in an ignored recovery folder.
- Verify native and packaged screens, native library origins, action races, file
  integrity, read-once privacy, repeated logo painting and full tray lifecycle.

## 0.2.1

- Paint the sidebar logo directly with CSS, removing its image texture path;
  disable accelerated compositing and request shared-memory presentation.
- Embed the logo as the AppImage `.DirIcon` and launcher icon, with a sharp
  512px PNG generated from the canonical SVG. Use it for the tray/window too.
- Preserve contact edits typed during a pending save, avoid duplicate saves,
  and show unsaved-change feedback.
- Ignore stale network responses, improve the missing-device view, clear
  cross-view sidebar filters, and prevent stale chat requests stealing focus.
- Exercise repeated view/theme/window changes with pixel checks of the logo,
  and verify the packaged icon and existing UI parity during release builds.

## 0.2.0

- Preserve normal desktop typography and layout in the bundled AppImage;
  compare actual packaged screenshots and font metrics before releasing.
- Add a styled sidebar search clear button and useful empty-result feedback.
  Apply filtering consistently to contact requests and refine time/context-menu
  presentation. Keep conversation search shortcuts out of open dialogs.
- Remove the obsolete native Qt chat interface. Keep a small tray supervisor
  and the shared GTK/WebKit web desktop with close-to-tray renderer release.
- Consolidate packaging behind `scripts/build.sh`, keep only current AppImage
  and `.deb` releases in `dist`, and move build artifacts/reports to `.build`.
- Refresh usage, build, and architecture documentation around implemented
  behavior; remove old wrappers, duplicate releases and speculative notes.

The app retains dark/light/system themes, floating conversation search,
contact/message context menus, encrypted file sharing, grouped LAN inventory,
local message deletion, read-once messages, profile compatibility and retries.
