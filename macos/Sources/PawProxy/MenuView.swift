import MenubarKit
import PawProxyNative
import SwiftUI

struct MenuView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        MenubarPanel {
            // Details quote commands in `code` spans, shown monospaced.
            StatusHeader(
                tone: model.presentation.tone, headline: AttributedString(model.presentation.headline),
                detail: InlineCode.attributed(model.presentation.detail ?? ""))
            StatusRowsTile(model.statusRows)
            routesTile
            if let action = model.presentation.primaryAction {
                PrimaryActionButton(title(for: action)) { model.perform(action) }
            }
            if let message = model.message {
                PanelNotice(InlineCode.attributed(message), symbol: "info.circle")
            }
            MenuSection(quitTitle: "Quit Paw Proxy", quitHelp: "The proxy keeps running after you quit.") {
                Button("Restart proxy") { model.restart() }
                    .disabled(model.isDevelopment && model.health == nil)
                Button("Open logs") { model.openLogs() }
                if model.registration == .enabled || model.registration == .requiresApproval {
                    Button("Turn proxy off") { model.turnOff() }
                }
                OpenSettingsButton()
            }
            VersionFooter()
        }
        .onAppear { model.refresh() }
    }

    private var routesTile: some View {
        VStack(alignment: .leading, spacing: 0) {
            TileHeader("Routes")
            if model.rows.isEmpty {
                Text(
                    model.health == nil ? "No routes." : "No routes yet. Start one with up <command>, e.g. up bun dev."
                )
                .font(.callout).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 12).padding(.vertical, 4)
            } else {
                ScrollView {
                    VStack(alignment: .leading, spacing: 0) {
                        ForEach(model.rows) { row in RouteRowView(row: row, model: model) }
                    }
                }
                .frame(maxHeight: 280)
            }
        }
        .groupedTile()
    }

    private func title(for action: StatusPresentation.Action) -> String {
        switch action {
        case .runSetup: return "Copy Setup Command"
        case .openLoginItems: return "Open Login Items"
        case .turnOn: return "Turn On Proxy"
        case .restart: return model.presentation.tone == .update ? "Restart to Finish Update" : "Restart Proxy"
        case .openLogs: return "Open Logs"
        }
    }
}

struct RouteRowView: View {
    var row: RouteRow
    @ObservedObject var model: AppModel

    var body: some View {
        ListRowButton(
            row.host, subtitle: row.project.isEmpty ? "→ \(row.target)" : "→ \(row.target) · \(row.project)",
            value: row.heartbeatAge
        ) { model.open(row) }
        .opacity(row.isStale ? 0.5 : 1)
        .help(
            row.isStale
                ? "No heartbeat recently — the route may be expiring." : "Open \(row.url?.absoluteString ?? row.host)"
        )
        .contextMenu {
            Button("Open") { model.open(row) }
            Button("Copy URL") { model.copyURL(row) }
        }
    }
}
