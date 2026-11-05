import MenubarKit
import Testing
@testable import PawProxyNative

private let ok = Health(status: "ok", version: "2026.11.1", build: "3", uptime: "1h")

private func status(setup: Bool = true, reg: RegistrationState = .enabled, health: Health? = ok,
                    routes: Int = 2, version: String = "2026.11.1", build: String = "3") -> StatusPresentation {
    .make(setupComplete: setup, registration: reg, health: health, routeCount: routes,
          bundleVersion: version, bundleBuild: build)
}

@Test func setupIncompleteWinsOverEverything() {
    let s = status(setup: false, reg: .requiresApproval, health: nil)
    #expect(s.tone == .error)
    #expect(s.primaryAction == .runSetup)
}

@Test func approvalBeatsHealth() {
    #expect(status(reg: .requiresApproval).primaryAction == .openLoginItems)
    #expect(status(reg: .requiresApproval, health: nil).primaryAction == .openLoginItems)
}

@Test func notAnswering() {
    #expect(status(reg: .enabled, health: nil) == .init(tone: .error, headline: "Proxy not responding",
        detail: "The daemon is registered but its socket does not answer.", primaryAction: .restart))
    #expect(status(reg: .notRegistered, health: nil).primaryAction == .turnOn)
    #expect(status(reg: .notFound, health: nil).tone == .off)
    #expect(status(reg: .disabled, health: nil).primaryAction == .openLogs)
    // A non-ok health status counts as not answering.
    #expect(status(health: Health(status: "starting", version: "x", uptime: "")).tone == .error)
}

@Test func updatePendingBeatsLegacyAndRunning() {
    let s = status(reg: .notRegistered, version: "2026.11.2", build: "1")
    #expect(s.tone == .update)
    #expect(s.primaryAction == .restart)
    #expect(s.detail == "Running 2026.11.1 (build 3); installed 2026.11.2 (build 1).")
}

@Test func legacyAgentOffersMigration() {
    let s = status(reg: .notRegistered)
    #expect(s.tone == .warning)
    #expect(s.primaryAction == .turnOn)
}

@Test func runningHeadlines() {
    #expect(status(routes: 0) == .init(tone: .ok, headline: "Running — no routes"))
    #expect(status(routes: 1).headline == "Running — 1 route")
    #expect(status(reg: .disabled, routes: 5) == .init(tone: .ok, headline: "Running — 5 routes"))
}

/// Every paw-proxy state maps to one MenubarKit tone; only running is badge-free.
@Test func statesMapOntoMenubarKitTones() {
    let cases: [(StatusPresentation, StatusTone, String)] = [
        (status(), .ok, "Running"),
        (status(setup: false), .error, "Setup needed"),
        (status(reg: .requiresApproval), .warning, "Waiting for approval"),
        (status(reg: .enabled, health: nil), .error, "Proxy not responding"),
        (status(reg: .notRegistered, health: nil), .off, "Proxy is off"),
        (status(version: "2026.11.2"), .update, "Restart to finish update"),
        (status(reg: .notRegistered), .warning, "Running — 2 routes"),
    ]
    for (presentation, tone, word) in cases {
        let menubar = presentation.status
        #expect(menubar.tone == tone)
        #expect(menubar.word == word)
        #expect(menubar.headline == presentation.headline)
        #expect(menubar.tone.needsAttention == (tone != .ok))
    }
}

@Test func headerDetailDropsCodeMarkup() {
    #expect(status(setup: false).status.detail
        == "Run sudo paw-proxy setup once to trust the local CA and configure DNS.")
    #expect(status().status.detail == "")
}

@Test func voiceOverLabelNamesTheState() {
    #expect(TrayAccessibility.label(appName: "Paw Proxy", statusWord: status(setup: false).status.word)
        == "Paw Proxy, Setup needed")
}
