import MenubarKit

/// The status tile under the panel header: one row per moving part, each
/// with its own tone symbol.
public enum PanelRows {
    public static func make(setupComplete: Bool, registration: RegistrationState, health: Health?) -> [MenubarStatusRow] {
        let answering = health?.status == "ok"
        let setup = setupComplete
            ? MenubarStatusRow(title: "Setup", value: "CA and DNS ready", tone: .ok)
            : MenubarStatusRow(title: "Setup", value: "Not done", tone: .error)

        let item: MenubarStatusRow
        switch registration {
        case .enabled: item = .init(title: "Background item", value: "Allowed", tone: .ok)
        case .requiresApproval: item = .init(title: "Background item", value: "Needs approval", tone: .warning)
        // A daemon that answers without the app's item is the old LaunchAgent.
        case .notRegistered, .notFound:
            item = .init(title: "Background item", value: answering ? "Old LaunchAgent" : "Off",
                         tone: answering ? .warning : .off)
        case .disabled: item = .init(title: "Background item", value: "Dev build", tone: .off)
        }

        let daemon: MenubarStatusRow
        if let health, answering {
            let uptime = health.uptime.isEmpty ? "" : " · up \(health.uptime)"
            daemon = .init(title: "Daemon", value: "\(health.version)\(uptime)", tone: .ok)
        } else {
            daemon = .init(title: "Daemon", value: "Not answering", tone: registration == .enabled ? .error : .off)
        }
        return [setup, item, daemon]
    }
}
