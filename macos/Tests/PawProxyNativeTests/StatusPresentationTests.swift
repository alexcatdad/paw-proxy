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
