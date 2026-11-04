import PawProxyNative
import SwiftUI

struct MenuView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            header
            Divider()
            routeList
            if let message = model.message {
                Text(message).font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            }
            Divider()
            actions
        }
        .padding(12)
        .frame(width: 340)
        .onAppear { model.refresh() }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Circle().fill(toneColor).frame(width: 8, height: 8)
                Text(model.presentation.headline).font(.headline)
                Spacer()
                if model.isDevelopment {
                    Text("DEV").font(.caption2.bold()).padding(.horizontal, 4)
                        .background(.orange.opacity(0.25), in: RoundedRectangle(cornerRadius: 3))
                }
            }
            if let detail = model.presentation.detail {
                Text(.init(detail)).font(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let action = model.presentation.primaryAction {
                Button(title(for: action)) { model.perform(action) }
                    .buttonStyle(.borderedProminent).controlSize(.small)
            }
        }
    }

    @ViewBuilder private var routeList: some View {
        if model.rows.isEmpty {
            Text(model.health == nil ? "No routes." : "No routes yet. Start one with `up <command>`, e.g. `up bun dev`.")
                .font(.caption).foregroundStyle(.secondary)
        } else {
            ScrollView {
                VStack(alignment: .leading, spacing: 2) {
                    ForEach(model.rows) { row in RouteRowView(row: row, model: model) }
                }
            }
            .frame(maxHeight: 320)
        }
    }

    private var actions: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Button("Restart proxy") { model.restart() }.disabled(model.isDevelopment && model.health == nil)
                Button("Open logs") { model.openLogs() }
                if model.registration == .enabled || model.registration == .requiresApproval {
                    Button("Turn proxy off") { model.turnOff() }
                }
            }
            if !model.isDevelopment {
                Toggle("Show in menu bar at login", isOn: Binding(
                    get: { model.showAtLogin }, set: { model.setShowAtLogin($0) }))
                    .toggleStyle(.checkbox)
            }
            HStack {
                Text(versionLine).font(.caption2).foregroundStyle(.tertiary)
                Spacer()
                Button("Quit menu bar icon") { model.quit() }
                    .help("The proxy keeps running after you quit.")
            }
        }
        .controlSize(.small)
    }

    private var versionLine: String {
        var s = "App \(model.bundleVersion)"
        if let h = model.health { s += " · daemon \(h.version), up \(h.uptime)" }
        return s
    }

    private var toneColor: Color {
        switch model.presentation.tone {
        case .ok: return .green
        case .warning, .update: return .orange
        case .error: return .red
        case .off: return .gray
        }
    }

    private func title(for action: StatusPresentation.Action) -> String {
        switch action {
        case .runSetup: return "Copy setup command"
        case .openLoginItems: return "Open Login Items"
        case .turnOn: return "Turn on proxy"
        case .restart: return model.presentation.tone == .update ? "Restart to finish update" : "Restart proxy"
        case .openLogs: return "Open logs"
        }
    }
}

struct RouteRowView: View {
    var row: RouteRow
    @ObservedObject var model: AppModel
    @State private var hovering = false

    var body: some View {
        Button { model.open(row) } label: {
            VStack(alignment: .leading, spacing: 1) {
                HStack {
                    Text(row.host).font(.system(.body, design: .monospaced))
                    Spacer()
                    Text(row.heartbeatAge).font(.caption2).foregroundStyle(.tertiary)
                }
                Text(row.project.isEmpty ? "→ \(row.target)" : "→ \(row.target) · \(row.project)")
                    .font(.caption).foregroundStyle(.secondary)
            }
            .padding(.vertical, 4).padding(.horizontal, 6)
            .contentShape(Rectangle())
            .background(hovering ? Color.primary.opacity(0.08) : .clear, in: RoundedRectangle(cornerRadius: 5))
            .opacity(row.isStale ? 0.5 : 1)
        }
        .buttonStyle(.plain)
        .onHover { hovering = $0 }
        .help(row.isStale ? "No heartbeat recently — the route may be expiring." : "Open \(row.url?.absoluteString ?? row.host)")
        .contextMenu {
            Button("Open") { model.open(row) }
            Button("Copy URL") { model.copyURL(row) }
        }
    }
}
