import Testing
@testable import PawProxyNative

@Test(arguments: [
    // bundle version, bundle build, daemon version, daemon build, expected
    ("2026.11.1", "3", "2026.11.1", "3", UpdateState.current),
    ("2026.11.2", "1", "2026.11.1", "3", .restartNeeded(running: "2026.11.1 (build 3)")),
    ("2026.11.1", "4", "2026.11.1", "3", .restartNeeded(running: "2026.11.1 (build 3)")),
    ("2026.11.1", "3", "2026.11.1", "", .current),          // daemon predates build reporting
    ("2026.11.1", "3", "1.12.3", "", .restartNeeded(running: "1.12.3")), // legacy formula daemon
    ("dev", "1", "2026.11.1", "3", .current),               // dev bundle never nags
    ("2026.11.1", "3", "dev", "", .current),
    ("2026.11.1", "3", "ci-abc1234", "", .current),
])
func updateState(_ c: (String, String, String, String, UpdateState)) {
    #expect(UpdateState.compare(bundleVersion: c.0, bundleBuild: c.1, daemonVersion: c.2, daemonBuild: c.3) == c.4)
}
