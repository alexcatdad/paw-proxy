import Foundation
import Testing

@testable import PawProxyNative

final class FakeRegistrar: Registrar, @unchecked Sendable {
    var current: RegistrationState = .notRegistered
    var onRegister: RegistrationState = .enabled
    var fails = false
    var log: [String]

    init(log: [String] = []) { self.log = log }
    var state: RegistrationState { current }
    func register() throws {
        log.append("register")
        if fails { throw LaunchctlError("SMAppService error 1") }
        current = onRegister
    }
    func unregister() throws { log.append("unregister"); current = .notRegistered }
    func openLoginItems() {}
}

final class FakeLaunchctl: LaunchctlRunner, @unchecked Sendable {
    var loaded: Set<String> = []
    var log: [String] = []
    func pid(_ label: String) -> Int32? { loaded.contains(label) ? 1 : nil }
    func bootout(_ label: String) throws { log.append("bootout \(label)"); loaded.remove(label) }
    func bootstrap(plist: URL) throws { log.append("bootstrap") }
    func kickstart(_ label: String) throws { log.append("kickstart \(label)") }
}

struct AgentFixture {
    let registrar = FakeRegistrar()
    let launchctl = FakeLaunchctl()
    let paths: Paths
    let agent: DaemonAgent

    init(legacy: Bool = false) throws {
        let home = FileManager.default.temporaryDirectory.appendingPathComponent("pp-agent-\(UUID().uuidString)")
        paths = Paths(home: home, resolverDir: home)
        agent = DaemonAgent(paths: paths, registrar: registrar, launchctl: launchctl)
        if legacy {
            try FileManager.default.createDirectory(
                at: agent.legacyPlist.deletingLastPathComponent(),
                withIntermediateDirectories: true)
            try Data("<plist/>".utf8).write(to: agent.legacyPlist)
            launchctl.loaded.insert(Identity.legacyLabel)
        }
    }

    var legacyOnDisk: Bool { FileManager.default.fileExists(atPath: agent.legacyPlist.path) }
}

@Test func turnOnCleanInstall() throws {
    let f = try AgentFixture()
    #expect(f.agent.turnOn().0 == .registered)
    #expect(f.launchctl.log.isEmpty)
    #expect(f.registrar.log == ["register"])
}

@Test func turnOnRemovesLegacyBeforeRegistering() throws {
    let f = try AgentFixture(legacy: true)
    #expect(f.agent.turnOn().0 == .registered)
    #expect(f.launchctl.log == ["bootout dev.paw-proxy"])
    #expect(!f.legacyOnDisk)
    #expect(!f.launchctl.isLoaded(Identity.legacyLabel))
}

@Test func turnOnPendingApproval() throws {
    let f = try AgentFixture(legacy: true)
    f.registrar.onRegister = .requiresApproval
    let (outcome, detail) = f.agent.turnOn()
    #expect(outcome == .requiresApproval)
    #expect(detail?.contains("Login Items") == true)
    #expect(!f.legacyOnDisk)
}

@Test func turnOnFailureSuggestsLegacyFallback() throws {
    let f = try AgentFixture(legacy: true)
    f.registrar.fails = true
    let (outcome, detail) = f.agent.turnOn()
    #expect(outcome == .failed)
    #expect(detail?.contains("--legacy-launchagent") == true)
}

@Test func turnOnAgentMissingFromBundle() throws {
    let f = try AgentFixture()
    f.registrar.onRegister = .notFound
    #expect(f.agent.turnOn().0 == .failed)
}

@Test func turnOnIsIdempotent() throws {
    let f = try AgentFixture()
    f.registrar.current = .enabled
    #expect(f.agent.turnOn().0 == .registered)
    #expect(f.registrar.log.isEmpty)
}

@Test func devBuildNeverTouchesAnything() throws {
    let f = try AgentFixture(legacy: true)
    let dev = DaemonAgent(paths: f.paths, registrar: DisabledRegistrar(), launchctl: f.launchctl)
    #expect(dev.turnOn() == (.failed, "Dev build — registration disabled"))
    #expect(f.launchctl.log.isEmpty)
    #expect(f.legacyOnDisk)
}

@Test func turnOffAndStatus() throws {
    let f = try AgentFixture()
    #expect(f.agent.turnOff().0 == .notRegistered)
    #expect(f.agent.status().0 == .notRegistered)
    f.registrar.current = .enabled
    #expect(f.agent.status().0 == .registered)
    #expect(f.agent.turnOff().0 == .unregistered)
    #expect(f.registrar.current == .notRegistered)
}

@Test func parsesLaunchctlPID() {
    #expect(SystemLaunchctl.parsePID("gui/501/x = {\n\tstate = running\n\tpid = 4242\n}") == 4242)
    #expect(SystemLaunchctl.parsePID("gui/501/x = {\n\tstate = not running\n}") == 0)
}
