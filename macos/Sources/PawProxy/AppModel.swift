import AppKit
import Foundation
import PawProxyNative

@MainActor
final class AppModel: ObservableObject {
    @Published private(set) var presentation = StatusPresentation(tone: .off, headline: "Checking…")
    @Published private(set) var rows: [RouteRow] = []
    @Published private(set) var health: Health?
    @Published private(set) var registration: RegistrationState = .notRegistered
    @Published private(set) var showAtLogin = false
    @Published var message: String?

    let isDevelopment: Bool
    let bundleVersion: String
    let bundleBuild: String

    private let services: Services
    private var paths: Paths { services.paths }
    private var revision = ObservationRevision()
    private var timer: Timer?

    init(services: Services = .live()) {
        self.services = services
        let info = Bundle.main.infoDictionary ?? [:]
        isDevelopment = services.isDevelopment
        bundleVersion = info["CFBundleShortVersionString"] as? String ?? "dev"
        bundleBuild = info["CFBundleVersion"] as? String ?? ""

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
        rows = RouteRow.rows(routes, now: Date())
        registration = services.registrar.state
        showAtLogin = services.loginItem?.isEnabled ?? false
        presentation = .make(setupComplete: setupComplete, registration: registration,
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
            services.registrar.openLoginItems()
        case .turnOn:
            turnOn()
        case .restart:
            restart()
        case .openLogs:
            openLogs()
        }
    }

    /// Replaces the legacy agent (if any) with the app's agent.
    func turnOn() {
        change("Turning on…") { $0.turnOn() }
    }

    func turnOff() {
        change("Turning off…") { $0.turnOff() }
    }

    private func change(_ headline: String, _ op: @escaping @Sendable (DaemonAgent) -> (Desktop.Outcome, String?)) {
        guard !isDevelopment else { message = DisabledRegistrar.DevBuild().description; return }
        revision.bump()
        presentation = StatusPresentation(tone: presentation.tone, headline: headline)
        let agent = services.agent
        Task.detached {
            let (_, detail) = op(agent)
            await MainActor.run { [weak self] in
                guard let self else { return }
                self.message = detail
                self.pollAfterChange(remaining: 20)
            }
        }
    }

    func setShowAtLogin(_ enabled: Bool) {
        guard let item = services.loginItem else { return }
        do {
            try item.setEnabled(enabled)
        } catch {
            message = "Could not change the login item: \(error.localizedDescription)"
        }
        showAtLogin = item.isEnabled
    }

    func restart() {
        revision.bump()
        presentation = StatusPresentation(tone: presentation.tone, headline: "Restarting…")
        let launchctl = services.launchctl
        Task.detached {
            let error = Self.kickstart(launchctl)
            await MainActor.run { [weak self] in
                guard let self else { return }
                if let error { self.message = error }
                self.pollAfterChange(remaining: 20)
            }
        }
    }

    /// Restart through launchctl (SMAppService has no restart call): the app
    /// agent if loaded, otherwise the legacy one.
    nonisolated static func kickstart(_ launchctl: LaunchctlRunner) -> String? {
        for label in [Identity.daemonLabel, Identity.legacyLabel] where launchctl.isLoaded(label) {
            do { try launchctl.kickstart(label); return nil } catch { return "Restart failed: \(error)" }
        }
        return "No paw-proxy daemon is installed."
    }

    private func pollAfterChange(remaining: Int) {
        refresh()
        guard remaining > 0 else { return }
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.25) { [weak self] in
            guard let self else { return }
            if self.health == nil || self.presentation.headline.hasSuffix("…") {
                self.pollAfterChange(remaining: remaining - 1)
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
