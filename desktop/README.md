# DevCore.app — desktop shell

DevCore's personal desktop UI. This is the **Path B** shell (buildspec §9): a
native macOS app whose window hosts the DevCore design prototype in a
`WKWebView`. It is the fast route to a working control surface; a full SwiftUI
nativization is a later pass.

## Layout

- `Shell/main.swift` — the macOS app: a window plus a `WKWebView`, with a custom
  URL-scheme handler that serves the web assets out of the app bundle.
- `Shell/Info.plist` — the app bundle's metadata.
- `web/` — the design prototype (HTML/CSS/JSX), from the Claude Design handoff
  bundle. Served to the web view unmodified.
- `build.sh` — compiles the shell and assembles `build/DevCore.app`.

## Build & run

```
./build.sh
open build/DevCore.app
```

React, Babel, and the web fonts load from a CDN, so the first launch needs the
network. Vendoring them locally — for fully offline use — is a follow-up.
