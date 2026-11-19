# Using SpearHead

## Conversations and contacts

Open SpearHead and choose your broadcast name. Use **Add a person** to choose a
nearby SpearHead peer or paste an invite/address. The other person accepts the
request. Compare identity words over a trusted channel to verify the contact.

The sidebar selector switches between **Conversations**, **Contacts**, and
**Network**. Contacts pins your profile first. A contact's assigned name is local
and separate from their broadcast name. The details pane includes device,
platform, last online time, address, identity words, and editable notes.

The sidebar search filters the selected list; its clear button restores the
list. In a conversation, Ctrl+F opens the floating message search. Escape or the
round close button closes it. Clicking the search button again also closes it;
clicking the sidebar selector again closes its menu. Right-click a conversation to view its contact;
right-click a message to copy text or delete your local copy. Local deletion
also cancels queued retries, but cannot delete a copy already delivered elsewhere.

Use the attachment button to share a file with an accepted contact. Files use
the encrypted direct connection, include an integrity hash, and are limited to
256 MiB. Read-once messages are revealed once locally; expiration and retention
remove message bodies according to the configured policy.

## Appearance and tray

Settings offers **Light**, **Dark**, and **System**, plus adjustable text size.
Appearance persists between launches. **Close to system tray** is checked by
default: closing the window keeps the service and tray running and releases the
native QML window process. Unsent drafts and contact edits are retained in RAM for
reopening; they are not saved as desktop preferences. Uncheck this setting to fully
stop the app when closing its window. The choice saves automatically between
launches. Closing waits for an active send, save or file transfer to finish.
Choose **Quit SpearHead** from the tray to stop everything at any time. Launching the same
profile again presents its existing window rather than starting a second service.

## Network view

Network lists discovered devices and provides LAN information for active
interfaces, gateway, DNS, and scan coverage. Your device groups its interface
addresses into one row. Signed peer identity, MAC addresses, and service UUIDs
help merge duplicate sightings. Generic device names/types are best guesses
based on mDNS, SSDP, reverse DNS, vendor information, and visible services.

Discovery only sees reachable or announcing devices. Sleeping devices,
isolated Wi-Fi clients, VPNs, firewalls, and large subnets can limit coverage;
this is not a complete inventory of every device connected to a router.

## Installation and troubleshooting

Quit any older running copy from its tray before launching a new release.

For the AppImage:

```sh
chmod +x spearhead-linux-amd64.AppImage
./spearhead-linux-amd64.AppImage
# If FUSE is unavailable:
./spearhead-linux-amd64.AppImage --appimage-extract-and-run
```

For the Debian installer:

```sh
sudo apt install ./spearhead_0.3.4_amd64.deb
spearhead
```

Default ports are TCP 7777 for peer traffic, UDP 7778 for discovery, and loopback
7780 for the authenticated UI. Allow peer traffic on your trusted LAN and check
client isolation if nearby peers cannot communicate. `spearhead doctor` reports
configuration issues. UI ports may be reassigned if occupied.

Data normally lives in `~/.config/spearhead/`; an existing legacy `racket`
profile is retained for upgrades. `--home DIR` or `SPEARHEAD_HOME` selects a
separate profile. Builds and cleanup do not touch these directories.
Back up the whole profile to retain identity, contacts, and history.

The command-line interface remains available through the AppImage or installed
`spearhead`: run `help` for commands, or `listen` for a headless service.
Do not delete identity files to troubleshoot a UI issue: that changes who you are
to your contacts. Linux profile contents are not encrypted at rest.
