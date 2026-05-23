// main.swift — DevCore.app: the native macOS shell for DevCore's desktop UI.
//
// Depends on: AppKit, WebKit, and Foundation (system frameworks only); and
// the devcore-api binary bundled alongside the shell at Contents/MacOS/.
// Depended on by: nothing — this file is the application entry point.
// Why it exists: DevCore's desktop UI ships first as a native shell (buildspec
// Path B) — a real macOS window hosting the design prototype in a web view.
// Two responsibilities live here: (1) serve the prototype's static assets
// through a custom URL scheme so the page has a real web origin, and
// (2) launch the read-only devcore-api subprocess and hand its localhost
// port to the page so its views can fetch live DevCore state.

import AppKit
import Foundation
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

/// APIProcessError reports why the devcore-api subprocess could not be brought
/// up. It conforms to LocalizedError so the failure is legible if logged.
enum APIProcessError: LocalizedError {
    case binaryMissing(path: String)
    case launchFailed(cause: Error)
    case portNotAnnounced

    var errorDescription: String? {
        switch self {
        case let .binaryMissing(path):
            "devcore-api binary not found at \(path)"
        case let .launchFailed(cause):
            "launching devcore-api failed: \(cause.localizedDescription)"
        case .portNotAnnounced:
            "devcore-api did not announce a LISTENING:<port> line before timing out"
        }
    }
}

/// APIProcess runs the bundled devcore-api binary as a child process and
/// captures the port it binds to. The desktop window stays usable even when
/// the API is unreachable: api.jsx falls back to placeholders when the
/// `?api=` query is missing, so a launch failure degrades to mocks rather
/// than a blank view.
final class APIProcess {
    /// portAnnounceTimeout is how long start() waits for the LISTENING line
    /// before giving up. devcore-api binds to a kernel-chosen ephemeral port
    /// in well under a second on a healthy machine; five seconds is generous.
    private static let portAnnounceTimeout: TimeInterval = 5

    private let process = Process()
    private let stdoutPipe = Pipe()
    private(set) var port: Int?

    /// start launches the binary at binaryURL with the given arguments and
    /// returns the bound port. It blocks until devcore-api prints
    /// `LISTENING:<port>` on its stdout or the timeout elapses.
    func start(binaryURL: URL, arguments: [String]) throws -> Int {
        guard FileManager.default.isExecutableFile(atPath: binaryURL.path) else {
            throw APIProcessError.binaryMissing(path: binaryURL.path)
        }

        process.executableURL = binaryURL
        process.arguments = arguments
        process.standardOutput = stdoutPipe
        // Let stderr inherit the shell's, so launch errors are visible when
        // the app is started from a terminal.
        process.standardError = FileHandle.standardError

        do {
            try process.run()
        } catch {
            throw APIProcessError.launchFailed(cause: error)
        }

        guard let port = try readPortAnnouncement() else {
            process.terminate()
            throw APIProcessError.portNotAnnounced
        }
        self.port = port
        return port
    }

    /// stop sends SIGTERM to the child and waits for it to exit. Calling stop
    /// on a process that has already exited is a no-op.
    func stop() {
        guard process.isRunning else { return }
        process.terminate()
        process.waitUntilExit()
    }

    /// readPortAnnouncement reads from devcore-api's stdout until it sees a
    /// line matching `LISTENING:<port>` and returns the parsed port. Other
    /// output is discarded — this is a port handshake, not a log channel.
    private func readPortAnnouncement() throws -> Int? {
        let handle = stdoutPipe.fileHandleForReading
        let deadline = Date().addingTimeInterval(Self.portAnnounceTimeout)
        var buffer = ""

        while Date() < deadline {
            let chunk = handle.availableData
            if chunk.isEmpty {
                // The child has closed its stdout — likely because it crashed
                // before announcing. Surface that as a missing announcement.
                return nil
            }
            if let text = String(data: chunk, encoding: .utf8) {
                buffer.append(text)
            }
            if let line = extractAnnouncement(buffer) {
                return line
            }
        }
        return nil
    }

    /// extractAnnouncement returns the port from a `LISTENING:<port>` line if
    /// one is present in text. Other lines are ignored.
    private func extractAnnouncement(_ text: String) -> Int? {
        for line in text.split(separator: "\n") {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if trimmed.hasPrefix("LISTENING:") {
                let portString = trimmed.dropFirst("LISTENING:".count)
                return Int(portString)
            }
        }
        return nil
    }
}

/// AppDelegate builds the single application window on launch and owns the
/// devcore-api subprocess.
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var window: NSWindow?
    private let apiProcess = APIProcess()

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

        let apiQuery = startAPIProcess(bundleResources: resources)
        guard let entry = URL(string: "\(schemeName)://app/\(entryPoint)\(apiQuery)") else {
            fatalError("DevCore.app: could not form the entry-point URL")
        }
        webView.load(URLRequest(url: entry))

        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationWillTerminate(_: Notification) {
        // Tear down the API subprocess so it does not outlive the UI.
        apiProcess.stop()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_: NSApplication) -> Bool {
        true
    }

    /// startAPIProcess launches devcore-api and returns the query suffix the
    /// page should be loaded with — either `?api=http://127.0.0.1:<port>` on
    /// success, or an empty string on failure (the page then renders with
    /// placeholder data instead of a blank view).
    private func startAPIProcess(bundleResources: URL) -> String {
        // The binary sits beside this executable in Contents/MacOS/.
        let binary = bundleResources
            .deletingLastPathComponent()
            .appendingPathComponent("MacOS/devcore-api")

        // Find the DevCore repo by walking up from the .app looking for the
        // marker file. This avoids hardcoding a relative offset between the
        // built .app and the repo root.
        guard let repoRoot = locateRepoRoot(startingAt: bundleResources) else {
            let reason = "could not locate the DevCore repo "
                + "(no devcore.config.yaml found above the .app)"
            FileHandle.standardError.write(Data(
                "DevCore.app: API unavailable — \(reason)\n".utf8,
            ))
            return ""
        }
        let episodicDB = repoRoot.appendingPathComponent(".devcore/state/episodic.sqlite").path
        let canonicalDir = repoRoot.appendingPathComponent(".devcore/memory").path

        do {
            let arguments = ["--episodic-db", episodicDB, "--canonical-dir", canonicalDir]
            let port = try apiProcess.start(binaryURL: binary, arguments: arguments)
            return "?api=http://127.0.0.1:\(port)"
        } catch {
            FileHandle.standardError.write(Data(
                "DevCore.app: API unavailable — \(error.localizedDescription)\n".utf8,
            ))
            return ""
        }
    }

    /// locateRepoRoot walks up from start looking for the devcore.config.yaml
    /// file that marks the DevCore repo root. Returns nil if no such file is
    /// reachable.
    private func locateRepoRoot(startingAt start: URL) -> URL? {
        var current = start.standardizedFileURL
        // 12 levels is far more than the longest plausible path between the
        // .app and the repo root; the loop terminates either way at "/".
        for _ in 0 ..< 12 {
            let marker = current.appendingPathComponent("devcore.config.yaml")
            if FileManager.default.fileExists(atPath: marker.path) {
                return current
            }
            let parent = current.deletingLastPathComponent()
            if parent.path == current.path {
                return nil
            }
            current = parent
        }
        return nil
    }
}

let application = NSApplication.shared
application.setActivationPolicy(.regular)
let appDelegate = AppDelegate()
application.delegate = appDelegate
application.run()
