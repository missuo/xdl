# XDL

Native macOS batch downloader for X/Twitter media timelines.
Objective-C + AppKit interface (Surge-style), Go backend running as a local
child process behind a bearer-authenticated localhost API.

## Features

- Fetch a user's media timeline (photos / videos / GIFs / text, retweets
  optional) and download everything with a concurrent pipeline: templated
  file naming, skip-existing, retries, metadata embedding (ExifTool) and
  GIF conversion (FFmpeg)
- Library of fetched accounts (SQLite) with refetch and local-file
  comparison — already-downloaded items are detected per file
- Browser extension (`extension/chrome`): send the profile you are viewing
  to the app with one click; it fetches, saves and downloads in the
  background with system notifications
- Menu-bar resident, dependency manager (xtractor / FFmpeg / ExifTool with
  Homebrew detection), auth token stored in the macOS Keychain

## Build

Requires Xcode 26+, Go, and [XcodeGen](https://github.com/yonaskolb/XcodeGen).

```sh
make core      # build the Go backend (build/xdl-core) — rerun after Go changes
make dev       # generate the Xcode project and build the app (ad-hoc signed)
make run       # build and launch
```

To sign: `make dev SIGNED=1 SIGN_ID="Developer ID Application: …"`.

## Credits

The extraction/download backend is derived from
[Twitter-X-Media-Batch-Downloader](https://github.com/afkarxyz/Twitter-X-Media-Batch-Downloader)
by afkarxyz (GPL-3.0). Timeline extraction uses its companion
[xtractor](https://github.com/afkarxyz/xtractor-binaries) binary.

## License

GPL-3.0 — see [LICENSE](LICENSE).
