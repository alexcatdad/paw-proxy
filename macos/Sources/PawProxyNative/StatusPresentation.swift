/// The one status model behind the menu-bar icon and the menu header.
public struct StatusPresentation: Equatable, Sendable {
    public enum Tone: Equatable, Sendable { case ok, warning, error, off, update }

    public enum Action: Equatable, Sendable {
        case runSetup        // copy `sudo paw-proxy setup` and explain
        case openLoginItems  // approve the background item
        case turnOn          // register the daemon (migrating the legacy agent)
        case restart         // kickstart the daemon
        case openLogs
    }

    public var tone: Tone
    public var headline: String
    public var detail: String?
    public var primaryAction: Action?

    public init(tone: Tone, headline: String, detail: String? = nil, primaryAction: Action? = nil) {
        self.tone = tone
        self.headline = headline
        self.detail = detail
        self.primaryAction = primaryAction
    }

    /// Precedence (first match wins): setup incomplete, needs approval,
    /// daemon not answering, update pending, legacy agent, running.
    public static func make(setupComplete: Bool,
                            registration: RegistrationState,
                            health: Health?,
                            routeCount: Int,
                            bundleVersion: String,
                            bundleBuild: String) -> StatusPresentation {
        if !setupComplete {
            return .init(tone: .error, headline: "Setup needed",
                         detail: "Run `sudo paw-proxy setup` once to trust the local CA and configure DNS.",
                         primaryAction: .runSetup)
        }
        if registration == .requiresApproval {
            return .init(tone: .warning, headline: "Waiting for approval",
                         detail: "Allow Paw Proxy in System Settings › General › Login Items.",
                         primaryAction: .openLoginItems)
        }

        guard let health, health.status == "ok" else {
            switch registration {
            case .enabled:
                return .init(tone: .error, headline: "Proxy not responding",
                             detail: "The daemon is registered but its socket does not answer.",
                             primaryAction: .restart)
            case .notRegistered, .notFound:
                return .init(tone: .off, headline: "Proxy is off", primaryAction: .turnOn)
            case .disabled:
                return .init(tone: .off, headline: "Daemon not running",
                             detail: "Dev build — start the installed daemon to see routes.",
                             primaryAction: .openLogs)
            case .requiresApproval: // unreachable: handled above
                return .init(tone: .warning, headline: "Waiting for approval", primaryAction: .openLoginItems)
            }
        }

        if case .restartNeeded(let running) = UpdateState.compare(
            bundleVersion: bundleVersion, bundleBuild: bundleBuild,
            daemonVersion: health.version, daemonBuild: health.build) {
            return .init(tone: .update, headline: "Restart to finish update",
                         detail: "Running \(running); installed \(UpdateState.describe(bundleVersion, bundleBuild)).",
                         primaryAction: .restart)
        }

        if registration == .notRegistered || registration == .notFound {
            // A healthy daemon without the app agent is the old CLI-written
            // LaunchAgent (pre-migration).
            return .init(tone: .warning, headline: running(routeCount),
                         detail: "Running from the old LaunchAgent. Move it to Paw Proxy to keep it attributed and up to date.",
                         primaryAction: .turnOn)
        }
        return .init(tone: .ok, headline: running(routeCount))
    }

    static func running(_ count: Int) -> String {
        switch count {
        case 0: return "Running — no routes"
        case 1: return "Running — 1 route"
        default: return "Running — \(count) routes"
        }
    }
}
