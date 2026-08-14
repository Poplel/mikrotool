# Mikrotool v2.1

Mikrotool is a Go/Fyne desktop utility for keeping a small list of MikroTik sites, opening them in WinBox, and creating short-lived WireGuard client sessions through RouterOS SSH.

## License

Copyright 2026 Elan Fergusson. Mikrotool is free and open-source software
licensed under the [GNU General Public License version 3 only](LICENSE)
(`GPL-3.0-only`). Individuals and businesses may use and modify it. Anyone who
distributes Mikrotool or a modified version must preserve the GPL terms and
make the corresponding source available. The complete license and no-warranty
notice are also compiled into the app's Licenses / About page on every platform.

GPLv3 permits charging for distribution, but every recipient keeps the right to
redistribute the software and source under the same license. Official Mikrotool
releases are provided free of charge.

## What it does

- Stores `IP address`, `site ID`, and `site name` records in a local CSV. Saving updates the case-insensitive site-ID match or creates a new record.
- Imports Mikrotool CSV files and WinBox 3 `.wbx` managed-router lists from Settings. For WBX imports, `host` maps to IP address, `group` maps to site ID, and `note` maps to site name; saved WinBox usernames and passwords are deliberately ignored.
- Exports the full site list as a portable CSV with `ip_address`, `site_id`, and `site_name` headers. A CSV written by an earlier version, with a `company_code` column, still imports and loads; the current header replaces it the next time the list is saved.
- Adds or updates a site from a `mikrotool:` link opened in a browser, and can immediately open WinBox or connect WireGuard to it. See [Links](#links).
- Copies a shareable link for any saved site from its right-click menu.
- Loads a record into the main fields with one click. Double-clicking any list value edits that value.
- Sorts sites in either direction by IP address, site ID, or site name; site-name sorting includes letter dividers and defaults to Z–A.
- Searches names, site IDs, and IP addresses from the main page, selecting and scrolling to the closest match.
- Deletes a saved site from its right-click menu after confirmation.
- Copies each main field independently.
- Stores the SSH password in macOS Keychain or Windows Credential Manager, never in the CSV or Fyne preferences.
- Automatically trusts and pins a router's first SSH host key in a private `known_hosts` file. A changed key is blocked and displays the expected and received SHA-256 fingerprints.
- Validates every site, credential, RouterOS value, executable path, and process argument before use; external commands are invoked directly without a shell except for strictly quoted, fixed macOS privilege-helper commands.
- Finds the first unused IPv4 address in the selected RouterOS WireGuard subnet, generates a fresh Curve25519 key and username-based random peer name, logs each connection/removal stage on RouterOS, and creates the peer.
- Cancels an in-progress WireGuard setup cleanly when the WireGuard button is pressed again, including removing a peer if setup had already created one.
- Starts a local full-tunnel WireGuard session. Disconnecting, closing Mikrotool, or detecting an externally stopped tunnel removes the matching router peer.
- Records only non-secret cleanup metadata so an interrupted session is removed on the next launch.
- Saves validated settings automatically and keeps a live, copyable private log in Settings, removing entries older than 24 hours.
- Includes the complete shared license and dependency audit inside both builds, behind the information `(i)` button in Settings, plus the Mikrotool v2.1 credit: Made by Elan Fergusson.

## Links

A `mikrotool:` link carries one site and opens it in Mikrotool from a browser,
an internal wiki, a ticket, or a chat message:

```
mikrotool://site?ip=203.0.113.10&id=ACME&name=Acme%20HQ
```

- `ip`, `id`, and `name` are required and hold the same three values as a saved
  site. Opening the link updates the case-insensitive site-ID match or creates a
  new record, exactly like the Save button, and loads it into the main fields.
- `connect` is optional and is either `winbox` or `wireguard`. A link may ask
  for one of them but never both; a link that asks for both is rejected. The
  bare forms `&winbox` and `&wireguard` work as well.
- Every value is validated with the rules the site editor uses, so a link can
  never store a record the application would reject.
- Right-click any saved site and choose **Copy Link** to put its link on the
  clipboard.

With `connect`, Mikrotool starts that connection as soon as the site is saved,
using the SSH credentials already in Settings, and no further confirmation is
asked for. Treat a Mikrotool link like any other link: opening one from an
untrusted page starts a real connection to whatever router that page names. A
link never carries a username or password and never changes Settings. A
`wireguard` link is ignored while a session is already connected or in progress,
so a link can never drop a live tunnel.

Registration is per platform:

- macOS: LaunchServices reads the `CFBundleURLTypes` entry in the app bundle
  when the app is installed, and delivers the link as an Apple Event to the
  running copy of Mikrotool, or launches it first.
- Windows: Mikrotool registers `HKCU\Software\Classes\mikrotool` on every start,
  which needs no administrator rights. A link starts a short-lived second
  process that hands the link to the running copy through the private inbox
  below and exits, so one instance keeps managing the site list and any tunnel.

## Local data

The OS user config directory contains:

- `Mikrotool/sites.csv` — non-secret site bookmarks, mode `0600` where supported.
- `Mikrotool/known_hosts` — pinned SSH public host keys, mode `0600`.
- `Mikrotool/active-session.json` — non-secret recovery metadata, present only while cleanup may be needed.
- `Mikrotool/actions.csv` — non-secret application actions from the last 24 hours, mode `0600` where supported.
- `Mikrotool/active/*.conf` — a private WireGuard config on macOS only while the tunnel is active, protected with mode `0600` and removed on disconnect.
- `Mikrotool/instance.lock` — an empty file whose lock marks the instance that owns this data directory.
- `Mikrotool/inbox/link-*` — a validated link waiting to be handed to that instance, mode `0600` and removed as soon as it is applied.

The password is held by the operating system credential manager under service `com.wirestar.mikrotool`.

## Requirements

- RouterOS 7 with SSH enabled and an enabled WireGuard interface with an IPv4 address.
- An SSH account allowed to read the WireGuard/interface information, write an info log entry, and add/remove WireGuard peers.
- Windows: WireGuard for Windows. Mikrotool uses the documented `/installtunnelservice` and `/uninstalltunnelservice` commands and requests administrator approval only while installing or removing the transient tunnel service.
- macOS on Apple Silicon: the packaged app includes the official `wg`, `wg-quick`, and `wireguard-go` helpers plus a compatible Bash runtime. No Homebrew installation is required. One temporary authorized session handles setup, disconnect, cancellation, and crash cleanup for each connection, so a second authorization is not needed when disconnecting.
- WinBox 4. On Windows, select any local `.exe` filename from the Settings file picker if WinBox is not installed in a standard location. On macOS, `WinBox.app` is located automatically in `/Applications`.

WinBox officially accepts the target, username, and password as local process arguments. Mikrotool invokes the executable directly without a shell and never logs those arguments. Be aware that same-user process-inspection tools may briefly be able to see them; this is a limitation of WinBox's documented automation interface.

## Development

```sh
go test -tags no_emoji ./...
fyne package -os darwin -icon Icon.png -name Mikrotool -release -tags no_emoji
packaging/macos/add-url-scheme.sh Mikrotool.app
CC=x86_64-w64-mingw32-gcc fyne package -os windows -icon Icon.png -name Mikrotool -release -tags no_emoji
```

`fyne package` writes `Info.plist` from its own template, which has no field for
a URL scheme, so `add-url-scheme.sh` adds the `CFBundleURLTypes` entry that
claims `mikrotool:`. Run it after packaging and before codesigning, because the
signature covers `Info.plist`. Windows needs no packaging step; the app
registers the scheme for the current user each time it starts.

Application ID: `com.wirestar.mikrotool`  
Version: `2.1.0`

## Release artifacts

- `releases/v2.1/Mikrotool-v2.1.0-macos-arm64.dmg` — Apple Silicon DMG.
- `releases/v2.1/Mikrotool-v2.1.0-windows-amd64.exe` — Windows x64 executable.
- `Mikrotool-v2.1-source/` — the complete corresponding source for both v2.1 builds, kept beside the earlier `Mikrotool-v2.0-source/` because the v2.0.0 binaries are still distributed.

Earlier releases stay where they were built: `releases/Mikrotool-v2.0.0-macos-arm64.dmg` and `releases/Mikrotool-v2.0.0-windows-amd64.exe`.

## macOS distribution

The Apple Silicon DMG contains `Mikrotool.app` and an Applications shortcut. The
complete corresponding source for the bundled WireGuard and Bash utilities is
stored inside the application bundle. The complete license inventory is compiled
into both applications and is available from the information `(i)` button in
Settings.
Drag Mikrotool to Applications; no separate package-manager installation is
needed. The release is signed with a Developer ID Application certificate, notarized by
Apple, and carries stapled notarization tickets on both the app and DMG.

See the in-app Licenses / About page for licenses and notices. The source
checksum inventory is also maintained in `packaging/macos/THIRD-PARTY-NOTICES.md`.

### macOS release sequence

Signing identity: the login keychain holds two valid identities with the same
`Developer ID Application: Elan Fergusson (UVD52RMS54)` name, so sign by SHA-1
hash rather than by name. Releases use `71AD237EF6BE28F7B1E08BAE61DA146FC47020D1`.

Notarization credentials are stored once in the login keychain as the
`mikrotool` profile (created with `xcrun notarytool store-credentials`), so
submissions need only `--keychain-profile mikrotool`. Rotating the app-specific
password invalidates the profile and requires storing it again.

```sh
ID=71AD237EF6BE28F7B1E08BAE61DA146FC47020D1
DMG=releases/vX.Y/Mikrotool-vX.Y.Z-macos-arm64.dmg

fyne package -os darwin -icon Icon.png -name Mikrotool --app-build <n> -release -tags no_emoji
packaging/macos/add-url-scheme.sh Mikrotool.app

# Bundle the arm64 helpers and their corresponding source.
mkdir -p Mikrotool.app/Contents/Resources/wireguard/bin Mikrotool.app/Contents/Resources/open-source
for f in bash wg wireguard-go; do lipo -thin arm64 build/macos-tools/$f -output Mikrotool.app/Contents/Resources/wireguard/bin/$f; done
cp build/macos-tools/wg-quick Mikrotool.app/Contents/Resources/wireguard/bin/
chmod 755 Mikrotool.app/Contents/Resources/wireguard/bin/*
cp -R build/upstream/. Mikrotool.app/Contents/Resources/open-source/

# Sign nested helpers before the bundle.
for f in bash wg wireguard-go; do codesign --force --options runtime --timestamp -s $ID Mikrotool.app/Contents/Resources/wireguard/bin/$f; done
codesign --force --options runtime --timestamp -s $ID Mikrotool.app

# Notarize, staple the app, then build the DMG around the stapled app.
hdiutil create -volname Mikrotool -srcfolder stage -ov -format UDZO "$DMG"   # stage holds Mikrotool.app + an Applications symlink
codesign --force --timestamp -s $ID "$DMG"
xcrun notarytool submit "$DMG" --keychain-profile mikrotool --wait
xcrun stapler staple Mikrotool.app
# Rebuild, re-sign, resubmit, and staple the DMG so both carry tickets.
xcrun stapler staple "$DMG"
xcrun stapler validate "$DMG" && spctl -a -t open --context context:primary-signature -vv "$DMG"
```

The app must be stapled as well as the DMG; a DMG-only ticket leaves the
installed app without one, which fails a first launch without network access.
`fyne package` rewrites `Build` in `FyneApp.toml` on every run, so pin it with
`--app-build` and reset the file, or the macOS and Windows artifacts end up with
different build numbers.
