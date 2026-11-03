import AppKit
import Foundation
import PawProxyNative

@MainActor
final class AppModel: ObservableObject {
    @Published private(set) var presentation = StatusPresentation(tone: .off, headline: "Checking…")
    @Published private(set) var rows: [RouteRow] = []
    @Published private(set) var health: Health?
    @Published var message: String?

    let isDevelopment: Bool
    let bundleVersion: String
    let bundleBuild: String

    private let paths = Paths()
    private let registrar: Registrar
    private var revision = ObservationRevision()
    private var timer: Timer?
    private var routes: [Route] = []

    init() {
        let info = Bundle.main.infoDictionary ?? [:]
        isDevelopment = Identity.isDevelopment(bundleID: Bundle.main.bundleIdentifier)
        bundleVersion = info["CFBundleShortVersionString"] as? String ?? "dev"
        bundleBuild = info["CFBundleVersion"] as? String ?? ""
        registrar = isDevelopment ? DisabledRegistrar() : AgentRegistrar()

        timer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.refresh() }
        }
        refresh()
    }

    /// Reads daemon state off the main thread; drops the reply if a user
    /// action happened while it was in flight.
    func refresh() {
        let started = revision.current
        let client = DaemonClient(socketPath: paths.socket.path, timeout: 2)
        let probe = SetupProbe(paths: paths)
        Task.detached {
            let health = try? client.health()
            let routes = health == nil ? [] : ((try? client.routes()) ?? [])
            let setupComplete = probe.isComplete
            await MainActor.run { [weak self] in
                guard let self, self.revision.accepts(started) else { return }
                self.apply(health: health, routes: routes, setupComplete: setupComplete)
            }
        }
    }

    private func apply(health: Health?, routes: [Route], setupComplete: Bool) {
        self.health = health
        self.routes = routes
        rows = RouteRow.rows(routes, now: Date())
        presentation = .make(setupComplete: setupComplete, registration: registrar.state,
                             health: health, routeCount: routes.count,
                             bundleVersion: bundleVersion, bundleBuild: bundleBuild)
    }

    // MARK: Actions

    func perform(_ action: StatusPresentation.Action) {
        switch action {
        case .runSetup:
            copy("sudo paw-proxy setup")
            message = "Copied `sudo paw-proxy setup` — run it in Terminal."
        case .openLoginItems:
            registrar.openLoginItems()
        case .turnOn:
            // Registration and migration from the legacy agent land with the
            // next phase; until then setup is the supported path.
            copy("sudo paw-proxy setup")
            message = "Copied `sudo paw-proxy setup` — run it in Terminal to turn the proxy on."
        case .restart:
            restart()
        case .openLogs:
            openLogs()
        }
    }

    func restart() {
        revision.bump()
        presentation = StatusPresentation(tone: presentation.tone, headline: "Restarting…")
        let uid = getuid()
        Task.detached {
            let result = Launchctl.kickstart(uid: uid)
            await MainActor.run { [weak self] in
                guard let self else { return }
                if case .failure(let error) = result { self.message = error.description }
                self.pollAfterRestart(remaining: 20)
            }
        }
    }

    private func pollAfterRestart(remaining: Int) {
        refresh()
        guard remaining > 0 else { return }
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.25) { [weak self] in
            guard let self else { return }
            if self.health == nil || self.presentation.headline == "Restarting…" {
                self.pollAfterRestart(remaining: remaining - 1)
            }
        }
    }

    func open(_ row: RouteRow) {
        if let url = row.url { NSWorkspace.shared.open(url) }
    }

    func copyURL(_ row: RouteRow) {
        if let url = row.url { copy(url.absoluteString) }
    }

    func openLogs() {
        NSWorkspace.shared.open(paths.log)
    }

    func quit() { NSApp.terminate(nil) }

    private func copy(_ s: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(s, forType: .string)
    }
}

/// Restart through launchctl: SMAppService has no restart call.
enum Launchctl {
    struct Failure: Error, CustomStringConvertible { var description: String }

    static func kickstart(uid: uid_t) -> Result<Void, Failure> {
        for label in [Identity.daemonLabel, Identity.legacyLabel] {
            let target = "gui/\(uid)/\(label)"
            guard run(["print", target]).status == 0 else { continue }
            let r = run(["kickstart", "-k", target])
            return r.status == 0 ? .success(()) : .failure(Failure(description: "Restart failed: \(r.output)"))
        }
        return .failure(Failure(description: "No paw-proxy daemon is installed."))
    }

    private static func run(_ args: [String]) -> (status: Int32, output: String) {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        p.arguments = args
        let pipe = Pipe()
        p.standardOutput = pipe
        p.standardError = pipe
        do { try p.run() } catch { return (-1, "\(error)") }
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        return (p.terminationStatus, String(decoding: data, as: UTF8.self))
    }
}
