import Foundation

/// Turns the app-owned daemon agent on and off. Turning it on removes the
/// CLI-written `dev.paw-proxy` LaunchAgent first (boot out, delete plist) so
/// the two never compete for ports 80/443, then registers the bundled agent.
/// There is no rollback: if registration fails, `sudo paw-proxy setup
/// --legacy-launchagent` puts the old agent back.
public struct DaemonAgent: Sendable {
    public var paths: Paths
    public var registrar: Registrar
    public var launchctl: LaunchctlRunner

    public init(paths: Paths, registrar: Registrar, launchctl: LaunchctlRunner) {
        self.paths = paths
        self.registrar = registrar
        self.launchctl = launchctl
    }

    public var legacyPlist: URL {
        paths.home.appendingPathComponent("Library/LaunchAgents/\(Identity.legacyLabel).plist")
    }

    static let approvalDetail = "Approve Paw Proxy in System Settings › General › Login Items"
    static let fallbackHint = "Run `sudo paw-proxy setup --legacy-launchagent` to go back to the old LaunchAgent."

    public func turnOn() -> (Desktop.Outcome, String?) {
        if registrar.state == .disabled { return (.failed, DisabledRegistrar.DevBuild().description) }

        // Remove the legacy agent.
        if launchctl.isLoaded(Identity.legacyLabel) {
            do { try launchctl.bootout(Identity.legacyLabel) } catch { return (.failed, "\(error)") }
        }
        try? FileManager.default.removeItem(at: legacyPlist)

        // Register the bundled agent (no-op when already enabled).
        if registrar.state != .enabled {
            do { try registrar.register() } catch {
                return (.failed, "Registration failed: \(error.localizedDescription). \(Self.fallbackHint)")
            }
        }
        switch registrar.state {
        case .enabled: return (.registered, nil)
        case .requiresApproval: return (.requiresApproval, Self.approvalDetail)
        case .notRegistered, .notFound, .disabled:
            return (.failed, "The daemon agent was not found in Paw Proxy.app. \(Self.fallbackHint)")
        }
    }

    public func turnOff() -> (Desktop.Outcome, String?) {
        switch registrar.state {
        case .disabled: return (.failed, DisabledRegistrar.DevBuild().description)
        case .notRegistered, .notFound: return (.notRegistered, nil)
        case .enabled, .requiresApproval:
            do { try registrar.unregister() } catch { return (.failed, error.localizedDescription) }
            return (.unregistered, nil)
        }
    }

    public func status() -> (Desktop.Outcome, String?) {
        switch registrar.state {
        case .enabled: return (.registered, nil)
        case .requiresApproval: return (.requiresApproval, Self.approvalDetail)
        case .notRegistered, .notFound: return (.notRegistered, nil)
        case .disabled: return (.failed, DisabledRegistrar.DevBuild().description)
        }
    }
}
