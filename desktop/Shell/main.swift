// main.swift — DevCore.app: the native macOS shell for DevCore's desktop UI.
//
// Depends on: AppKit and WebKit (system frameworks only).
// Depended on by: nothing — this file is the application entry point.
// Why it exists: DevCore's desktop UI ships first as a native shell (buildspec
// Path B) — a real macOS window hosting the design prototype in a web view. The
// prototype's assets are served through a custom URL scheme so the page has a
// real web origin and its relative asset loads resolve normally.

import AppKit
import WebKit

/// schemeName is the custom URL scheme the bundled web assets are served under.
/// A custom scheme — rather than file:// — gives the prototype a real web origin.
let schemeName = "devcore"

/// entryPoint is the prototype's HTML file, served when no specific path is asked.
let entryPoint = "DevCore.app.html"

/// AssetError reports why a web asset could not be served. It conforms to
/// LocalizedError so the underlying cause stays legible in WebKit's diagnostics.
enum AssetError: LocalizedError {
    case noURL
    case unreadable(path: String, cause: Error)

    var errorDescription: String? {
        switch self {
        case .noURL:
            "the asset request carried no URL"
        case let .unreadable(path, cause):
            "could not read web asset \(path): \(cause.localizedDescription)"
        }
    }
}

/// AssetSchemeHandler serves the bundled prototype files to the web view. It
/// maps a `devcore://app/<path>` request to `<Resources>/web/<path>`.
final class AssetSchemeHandler: NSObject, WKURLSchemeHandler {
    private let webRoot: URL

    init(webRoot: URL) {
        self.webRoot = webRoot
    }

    func webView(_: WKWebView, start task: WKURLSchemeTask) {
        guard let url = task.request.url else {
            task.didFailWithError(AssetError.noURL)
            return
        }

        var path = url.path
        while path.hasPrefix("/") {
            path.removeFirst()
        }
        if path.isEmpty { path = entryPoint }

        let file = webRoot.appendingPathComponent(path)
        let data: Data
        do {
            data = try Data(contentsOf: file)
        } catch {
            // Carry the real cause — a missing file, a permission failure, and a
            // corrupt file must stay distinguishable when debugging a blank view.
            task.didFailWithError(AssetError.unreadable(path: path, cause: error))
            return
        }

        let response = URLResponse(
            url: url,
            mimeType: Self.mimeType(forExtension: file.pathExtension),
            expectedContentLength: data.count,
            textEncodingName: "utf-8",
        )
        task.didReceive(response)
        task.didReceive(data)
        task.didFinish()
    }

    func webView(_: WKWebView, stop _: WKURLSchemeTask) {
        // Assets are served synchronously in start(); there is nothing to cancel.
    }

    /// mimeType maps a file extension to a content type. The set is small because
    /// the prototype ships only HTML, CSS, and JSX assets.
    private static func mimeType(forExtension ext: String) -> String {
        switch ext.lowercased() {
        case "html": "text/html"
        case "css": "text/css"
        case "js", "jsx": "application/javascript"
        default: "application/octet-stream"
        }
    }
}

/// AppDelegate builds the single application window on launch.
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var window: NSWindow?

    func applicationDidFinishLaunching(_: Notification) {
        guard let resources = Bundle.main.resourceURL else {
            fatalError("DevCore.app: the bundle has no Resources directory")
        }
        let webRoot = resources.appendingPathComponent("web")

        let configuration = WKWebViewConfiguration()
        configuration.setURLSchemeHandler(
            AssetSchemeHandler(webRoot: webRoot), forURLScheme: schemeName,
        )
        let webView = WKWebView(frame: .zero, configuration: configuration)

        // The design is 1280x820 and scales itself down to fit; open it at 1:1.
        let win = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1280, height: 820),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false,
        )
        win.title = "DevCore"
        win.titlebarAppearsTransparent = true
        win.titleVisibility = .hidden
        win.contentView = webView
        win.center()
        win.makeKeyAndOrderFront(nil)
        window = win

        guard let entry = URL(string: "\(schemeName)://app/\(entryPoint)") else {
            fatalError("DevCore.app: could not form the entry-point URL")
        }
        webView.load(URLRequest(url: entry))

        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_: NSApplication) -> Bool {
        true
    }
}

let application = NSApplication.shared
application.setActivationPolicy(.regular)
let appDelegate = AppDelegate()
application.delegate = appDelegate
application.run()
