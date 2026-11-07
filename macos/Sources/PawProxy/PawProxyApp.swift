import AppKit
import MenubarKit
import PawProxyNative
import SwiftUI

struct PawProxyApp: App {
    @StateObject private var model = AppModel()
    private let trayIcon = TrayIconLoader.load(
        resourcesDirectory: Bundle.main.resourceURL, baseName: "PawProxyTrayIcon")

    var body: some Scene {
        MenuBarExtra {
            MenuView(model: model)
        } label: {
            // Constant paw glyph; MenubarKit adds the badge for any non-OK
            // tone, the DEV caption and "Paw Proxy, <status>" for VoiceOver.
            TrayLabel(
                icon: trayIcon, fallbackSymbol: "pawprint.fill", status: model.presentation.status,
                appName: "Paw Proxy", isDevelopment: model.isDevelopment)
        }
        .menuBarExtraStyle(.window)

        Settings {
            SettingsView(model: model)
        }
    }
}

struct SettingsView: View {
    @ObservedObject var model: AppModel
    @AppStorage(NotificationPreferences.enabledKey) private var notify = false

    var body: some View {
        SettingsForm {
            // The section asks macOS for permission from the toggle, keeps
            // its note in step with System Settings, and adds "Play a sound";
            // the notifier reads both keys on every delivery.
            NotificationSettingsSection(
                notifier: model.notifier, toggleTitle: "Notify me when projects go live",
                explanation: "When up brings a project online or stops it. Silent unless you turn on the sound.",
                offersSound: true)
            Section("Startup") {
                Toggle(
                    "Launch at login",
                    isOn: Binding(get: { model.showAtLogin }, set: { model.setShowAtLogin($0) })
                )
                .disabled(model.isDevelopment)
                if model.isDevelopment {
                    SettingsNote("Dev builds never register a login item.")
                }
            }
            SettingsAboutSection(
                bundle: model.bundle, repository: URL(string: "https://github.com/alexcatdad/paw-proxy"))
        }
        // Start or stop polling the daemon's event queue with the toggle.
        .onChange(of: notify) { model.notificationsToggled($0) }
    }
}
