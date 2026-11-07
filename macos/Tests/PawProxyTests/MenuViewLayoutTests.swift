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

// MARK: Website screenshots

/// A registered, approved daemon agent that never calls SMAppService.
private struct FixtureRegistrar: Registrar {
    var state: RegistrationState
    func register() throws {}
    func unregister() throws {}
    func openLoginItems() {}
}

/// A login item that never touches the real ones. Off: offscreen windows are
/// never key, so a switched-on toggle would draw in the inactive grey.
private struct FixtureLoginItem: LoginItem {
    var isEnabled: Bool { false }
    func setEnabled(_ enabled: Bool) throws {}
}

/// An app bundle on disk whose Info.plist carries a release version, so the
/// panel footer and Settings show one instead of the test runner's.
private func fixtureBundle(version: String) throws -> Bundle {
    let app = FileManager.default.temporaryDirectory
        .appendingPathComponent("paw-proxy-screenshots-\(UUID().uuidString)/Paw Proxy.app", isDirectory: true)
    let contents = app.appendingPathComponent("Contents", isDirectory: true)
    try FileManager.default.createDirectory(at: contents, withIntermediateDirectories: true)
    let info: [String: Any] = [
        "CFBundleIdentifier": "com.alexcatdad.paw-proxy.screenshots", "CFBundleShortVersionString": version,
        "CFBundleVersion": version,
    ]
    try PropertyListSerialization.data(fromPropertyList: info, format: .xml, options: 0)
        .write(to: contents.appendingPathComponent("Info.plist"))
    return try #require(Bundle(url: app))
}

/// A release build's model with made-up projects, for the website. Never the
/// owner's real routes: everything here is fixture data.
@MainActor
private func screenshotModel(setupComplete: Bool = true) throws -> AppModel {
    let paths = Paths(home: FileManager.default.temporaryDirectory.appendingPathComponent("paw-proxy-screenshots"))
    let registrar = FixtureRegistrar(state: setupComplete ? .enabled : .notRegistered)
    let launchctl = SystemLaunchctl()
    let services = Services(
        paths: paths, isDevelopment: false, registrar: registrar, launchctl: launchctl,
        loginItem: FixtureLoginItem(), agent: DaemonAgent(paths: paths, registrar: registrar, launchctl: launchctl))
    let version = "2026.1107.1"
    let model = AppModel(
        services: services, notificationCenter: AllowedCenter(), startsPolling: false,
        bundle: try fixtureBundle(version: version))
    guard setupComplete else {
        model.apply(health: nil, routes: [], setupComplete: false)
        return model
    }
    let now = Date()
    func route(_ name: String, _ port: Int, _ dir: String, seconds: TimeInterval) -> Route {
        Route(
            name: name, upstream: "localhost:\(port)", dir: "/Users/dev/code/\(dir)",
            registered: now.addingTimeInterval(-3600), lastHeartbeat: now.addingTimeInterval(-seconds))
    }
    model.apply(
        health: Health(status: "ok", version: version, build: version, uptime: "3h12m5s"),
        routes: [
            route("myapp", 3000, "myapp", seconds: 1),
            route("api.myapp", 8080, "myapp", seconds: 4),
            route("myapp-feat-auth", 3001, "myapp-feat-auth", seconds: 9),
            route("docs", 4321, "docs", seconds: 24),  // past half the timeout: drawn as stale
        ],
        setupComplete: true)
    return model
}

/// Writes the website's screenshots when `PAWPROXY_SCREENSHOTS=<dir>` is set
/// (`task screenshots`): the panel running and before setup, and Settings,
/// each in light and dark.
@MainActor
@Test func writesWebsiteScreenshotsWhenAsked() throws {
    guard let dir = ProcessInfo.processInfo.environment["PAWPROXY_SCREENSHOTS"] else { return }
    let output = URL(fileURLWithPath: dir, isDirectory: true)
    try FileManager.default.createDirectory(at: output, withIntermediateDirectories: true)
    let running = try screenshotModel()
    let setup = try screenshotModel(setupComplete: false)
    for (name, appearance) in [("light", NSAppearance.Name.aqua), ("dark", .darkAqua)] {
        let shots: [(String, Data?)] = [
            ("panel", PanelSnapshot.png(of: MenuView(model: running), appearance: appearance)),
            ("panel-setup", PanelSnapshot.png(of: MenuView(model: setup), appearance: appearance)),
            (
                "settings",
                PanelSnapshot.png(
                    of: SettingsView(model: running), width: MenubarMetrics.settingsWidth,
                    chrome: .window(title: "Paw Proxy Settings"), appearance: appearance)
            ),
        ]
        for (file, png) in shots {
            let data = try #require(png, "rendering \(file)-\(name) failed")
            try data.write(to: output.appendingPathComponent("\(file)-\(name).png"))
        }
    }
}
