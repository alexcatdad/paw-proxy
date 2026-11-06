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
        Form {
            Toggle(
                "Show in menu bar at login",
                isOn: Binding(
                    get: { model.showAtLogin }, set: { model.setShowAtLogin($0) })
            )
            .disabled(model.isDevelopment)
            if model.isDevelopment {
                Text("Dev builds never register a login item.").font(.caption).foregroundStyle(.secondary)
            }

            Toggle("Notify me when projects go live", isOn: $notify)
                .onChange(of: notify) { model.notificationsToggled($0) }
            Text(notificationNote).font(.caption).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(20)
        .frame(width: 380)
        .onAppear { model.refreshNotificationPermission() }
    }

    private var notificationNote: String {
        if notify && model.notificationPermission == .denied {
            return "Notifications are blocked for Paw Proxy in System Settings › Notifications."
        }
        return "A silent notification when up brings a project online or stops it."
    }
}
