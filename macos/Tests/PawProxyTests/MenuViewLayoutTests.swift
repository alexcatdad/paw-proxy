import AppKit
import Foundation
import MenubarKit
import PawProxyNative
import SwiftUI
import Testing
import UserNotifications

@testable import PawProxy

/// UNUserNotificationCenter needs a real app bundle; the views only read the permission.
private struct AllowedCenter: NotificationCenterClient {
    func requestAuthorization() async throws -> Bool { true }
    func authorizationStatus() async -> UNAuthorizationStatus { .authorized }
    func add(_ notification: MenubarNotification, playSound: Bool) async throws {}
}

/// A model that never touches launchd, the daemon socket or the login items.
@MainActor
private func model(routes count: Int) -> AppModel {
    let paths = Paths(home: FileManager.default.temporaryDirectory.appendingPathComponent("paw-proxy-layout"))
    let registrar = DisabledRegistrar()
    let launchctl = SystemLaunchctl()
    let services = Services(
        paths: paths, isDevelopment: true, registrar: registrar, launchctl: launchctl, loginItem: nil,
        agent: DaemonAgent(paths: paths, registrar: registrar, launchctl: launchctl))
    let model = AppModel(services: services, notificationCenter: AllowedCenter(), startsPolling: false)
    let now = Date()
    let routes = (0..<count).map { index in
        Route(
            name: "project-\(index)", upstream: "localhost:\(3000 + index)", dir: "/Users/dev/project-\(index)",
            registered: now, lastHeartbeat: now)
    }
    model.apply(
        health: Health(status: "ok", version: "dev", uptime: "1m"), routes: routes, setupComplete: true)
    return model
}

/// The panel's height when the window proposes none, as `MenuBarExtra` can.
@MainActor
private func minimalHeight(of view: some View) -> CGFloat {
    let controller = NSHostingController(rootView: view)
    controller.view.frame = NSRect(x: 0, y: 0, width: MenubarMetrics.panelWidth, height: 900)
    controller.view.layoutSubtreeIfNeeded()
    return controller.sizeThatFits(in: CGSize(width: MenubarMetrics.panelWidth, height: 0)).height
}

@MainActor
@Test func routeListKeepsItsHeightWhenTheWindowProposesNone() {
    let one = minimalHeight(of: MenuView(model: model(routes: 1)))
    let many = minimalHeight(of: MenuView(model: model(routes: 15)))
    // 15 routes overflow the 280pt cap, one route is a single ~40pt row. The
    // list must claim the full cap rather than collapse, as a ScrollView with
    // only a max height does (both panels then measure the same).
    #expect(many - one >= 200, "panel with 15 routes is \(many)pt, with one \(one)pt")
}

@MainActor
@Test func rendersPanelAndSettingsWhenAsked() throws {
    guard let dir = ProcessInfo.processInfo.environment["PAWPROXY_RENDER_DIR"] else { return }
    let output = URL(fileURLWithPath: dir, isDirectory: true)
    try FileManager.default.createDirectory(at: output, withIntermediateDirectories: true)
    let model = model(routes: 15)
    for (name, appearance) in [("light", NSAppearance.Name.aqua), ("dark", .darkAqua)] {
        try render(MenuView(model: model), width: MenubarMetrics.panelWidth, appearance: appearance)
            .write(to: output.appendingPathComponent("panel-\(name).png"))
        try render(SettingsView(model: model), width: MenubarMetrics.settingsWidth, appearance: appearance)
            .write(to: output.appendingPathComponent("settings-\(name).png"))
    }
}

/// Lays `view` out at its fitting height in an offscreen window and returns a PNG.
@MainActor
private func render(_ view: some View, width: CGFloat, appearance: NSAppearance.Name) throws -> Data {
    let host = NSHostingView(rootView: view.frame(width: width).fixedSize(horizontal: false, vertical: true))
    let window = NSWindow(
        contentRect: NSRect(x: 0, y: 0, width: width, height: 900), styleMask: [.borderless], backing: .buffered,
        defer: false)
    window.appearance = NSAppearance(named: appearance)
    window.backgroundColor = .windowBackgroundColor
    window.contentView = host
    host.layoutSubtreeIfNeeded()
    host.setFrameSize(NSSize(width: width, height: host.fittingSize.height))
    host.layoutSubtreeIfNeeded()
    RunLoop.main.run(until: Date().addingTimeInterval(0.2))
    defer { window.contentView = nil }
    guard let rep = host.bitmapImageRepForCachingDisplay(in: host.bounds) else { throw RenderError() }
    host.cacheDisplay(in: host.bounds, to: rep)
    guard let png = rep.representation(using: .png, properties: [:]) else { throw RenderError() }
    return png
}

private struct RenderError: Error {}
