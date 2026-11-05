import MenubarKit
import Testing
@testable import PawProxyNative

private let ok = Health(status: "ok", version: "2026.11.1", build: "3", uptime: "1h")

@Test func healthyRowsAreAllOK() {
    let rows = PanelRows.make(setupComplete: true, registration: .enabled, health: ok)
    #expect(rows.map(\.title) == ["Setup", "Background item", "Daemon"])
    #expect(rows.map(\.tone) == [.ok, .ok, .ok])
    #expect(rows[2].value == "2026.11.1 · up 1h")
}

@Test func problemRowsCarryTheirOwnTone() {
    let silent = PanelRows.make(setupComplete: false, registration: .enabled, health: nil)
    #expect(silent.map(\.tone) == [.error, .ok, .error])
    #expect(silent[2].value == "Not answering")

    let off = PanelRows.make(setupComplete: true, registration: .notRegistered, health: nil)
    #expect(off.map(\.value) == ["CA and DNS ready", "Off", "Not answering"])
    #expect(off.map(\.tone) == [.ok, .off, .off])

    let legacy = PanelRows.make(setupComplete: true, registration: .notFound, health: ok)
    #expect(legacy[1] == MenubarStatusRow(title: "Background item", value: "Old LaunchAgent", tone: .warning))

    #expect(PanelRows.make(setupComplete: true, registration: .requiresApproval, health: nil)[1].tone == .warning)
    #expect(PanelRows.make(setupComplete: true, registration: .disabled, health: ok)[1].value == "Dev build")
}
