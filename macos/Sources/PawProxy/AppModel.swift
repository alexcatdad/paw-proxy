import AppKit
import Foundation
import MenubarKit
import PawProxyNative

@MainActor
final class AppModel: ObservableObject {
    @Published private(set) var presentation = StatusPresentation(tone: .off, headline: "Checking…")
    @Published private(set) var rows: [RouteRow] = []
    @Published private(set) var health: Health?
    @Published private(set) var registration: RegistrationState = .notRegistered
    @Published private(set) var showAtLogin = false
    @Published private(set) var setupComplete = true
    @Published var message: String?

    let isDevelopment: Bool
    let bundle: Bundle
    let bundleVersion: String
    let bundleBuild: String

    var statusRows: [MenubarStatusRow] {
        PanelRows.make(setupComplete: setupComplete, registration: registration, health: health)
    }

    private let services: Services
    private var paths: Paths { services.paths }
    private var revision = ObservationRevision()
    private var timer: Timer?

    // Route notifications: `up` queues events in the daemon; the app polls
    // them and posts from its own bundle (opt-in, de-duplicated, silent
    // unless the user turns on "Play a sound").
    let notifier: MenubarNotifier
    private let responder = MenubarNotificationResponder { click in
        guard !click.isDismissal, let url = RouteEventFeed.clickURL(click.userInfo) else { return }
        NSWorkspace.shared.open(url)
    }
    private var feed = RouteEventFeed(since: Date())
    private var eventTimer: Timer?
    private var eventPollInFlight = false

    /// - Parameters:
    ///   - notificationCenter: Tests pass a fake; `UNUserNotificationCenter`
    ///     needs a real app bundle.
    ///   - startsPolling: Off in tests, which set the state they lay out.
    ///   - bundle: Where the version shown in the panel and Settings comes
    ///     from; the screenshot test passes a fixture bundle.
    init(
        services: Services = .live(),
        notificationCenter: any NotificationCenterClient = SystemNotificationCenter(),
        startsPolling: Bool = true,
        bundle: Bundle = .main
    ) {
        self.services = services
        self.bundle = bundle
        notifier = RouteNotifications.notifier(center: notificationCenter)
        isDevelopment = services.isDevelopment
        bundleVersion = bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "dev"
        bundleBuild = bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? ""
        guard startsPolling else { return }

        timer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.refresh() }
        }
        refresh()

        responder.install()
        eventTimer = Timer.scheduledTimer(withTimeInterval: 3, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.pollRouteEvents() }
        }
        pollRouteEvents()
    }

    // MARK: Notifications

    private var notificationsEnabled: Bool {
        UserDefaults.standard.bool(forKey: NotificationPreferences.enabledKey)
    }

    /// From the Settings toggle, which also asks macOS for permission.
    func notificationsToggled(_ enabled: Bool) {
        // Off forgets the cursor; on starts listening from now.
        feed.reset(since: Date())
        if enabled { pollRouteEvents() }
    }

    /// Polls the daemon's event queue while notifications are on. Off, the
    /// cursor is forgotten so turning them on later does not replay a backlog.
    func pollRouteEvents() {
        guard notificationsEnabled else { feed.reset(since: Date()); return }
        guard !eventPollInFlight else { return }
        eventPollInFlight = true
        let client = DaemonClient(socketPath: paths.socket.path, timeout: 2)
        let after = feed.after
        Task.detached {
            let page = try? client.routeEvents(after: after)
            await MainActor.run { [weak self] in
                guard let self else { return }
                self.eventPollInFlight = false
                guard let page, self.notificationsEnabled else { return }
                let notes = self.feed.accept(page)
                guard !notes.isEmpty else { return }
                let notifier = self.notifier
                Task {
                    for note in notes { await notifier.deliver(note) }
                }
            }
        }
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

    func apply(health: Health?, routes: [Route], setupComplete: Bool) {
        self.health = health
        self.setupComplete = setupComplete
        rows = RouteRow.rows(routes, now: Date())
        registration = services.registrar.state
        showAtLogin = services.loginItem?.isEnabled ?? false
        presentation = .make(
            setupComplete: setupComplete, registration: registration,
            health: health, routeCount: routes.count,
            bundleVersion: bundleVersion, bundleBuild: bundleBuild)
    }

    // MARK: Actions

    func perform(_ action: StatusPresentation.Action) {
        switch action {
        case .runSetup:
            copy("sudo paw-proxy setup")
            message = "Copied “sudo paw-proxy setup”. Run it in Terminal."
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
