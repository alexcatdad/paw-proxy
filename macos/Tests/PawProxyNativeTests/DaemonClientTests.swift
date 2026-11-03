import Foundation
import Testing
@testable import PawProxyNative

@Test func parsesResponses() throws {
    let ok = Data("HTTP/1.0 200 OK\r\nContent-Type: application/json\r\n\r\n{\"a\":1}".utf8)
    #expect(try DaemonClient.parse(ok) == Data("{\"a\":1}".utf8))
    #expect(throws: DaemonClientError.httpStatus(429)) {
        try DaemonClient.parse(Data("HTTP/1.1 429 Too Many Requests\r\n\r\n".utf8))
    }
    #expect(throws: DaemonClientError.malformedResponse) { try DaemonClient.parse(Data("garbage".utf8)) }
    #expect(throws: DaemonClientError.malformedResponse) { try DaemonClient.parse(Data("SPDY 200\r\n\r\n".utf8)) }
}

@Test func missingSocketIsAConnectError() {
    let client = DaemonClient(socketPath: "/tmp/paw-proxy-does-not-exist-\(UUID().uuidString.prefix(8)).sock", timeout: 1)
    #expect(throws: DaemonClientError.connect(ENOENT)) { try client.health() }
}

@Test func rejectsOverlongSocketPath() {
    let client = DaemonClient(socketPath: "/tmp/" + String(repeating: "x", count: 200))
    #expect(throws: DaemonClientError.socketPathTooLong) { try client.health() }
}

/// Runs the Go fixture daemon (internal/api/testfixture) and talks to it over
/// a real Unix socket. Enabled with PAWPROXY_TEST_DAEMON=1; CI sets it so
/// the test is mandatory there.
@Test(.enabled(if: ProcessInfo.processInfo.environment["PAWPROXY_TEST_DAEMON"] == "1"))
func talksToTheGoDaemon() throws {
    let repo = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent().deletingLastPathComponent()
        .deletingLastPathComponent().deletingLastPathComponent()
    let dir = URL(fileURLWithPath: "/tmp/ppt-\(UUID().uuidString.prefix(8))")
    try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: dir) }

    // Build first so `go run` startup time does not count against the wait.
    let bin = dir.appendingPathComponent("fixture")
    let build = Process()
    build.executableURL = URL(fileURLWithPath: "/usr/bin/env")
    build.arguments = ["go", "build", "-o", bin.path, "./internal/api/testfixture"]
    build.currentDirectoryURL = repo
    try build.run()
    build.waitUntilExit()
    #expect(build.terminationStatus == 0)

    let socket = dir.appendingPathComponent("d.sock").path
    let daemon = Process()
    daemon.executableURL = bin
    daemon.arguments = ["-socket", socket, "-routes", "60"] // > 2 KiB of JSON
    try daemon.run()
    defer { daemon.terminate(); daemon.waitUntilExit() }

    let client = DaemonClient(socketPath: socket, timeout: 2)
    var health: Health?
    for _ in 0..<100 where health == nil {
        health = try? client.health()
        if health == nil { usleep(50_000) }
    }
    #expect(health == Health(status: "ok", version: "2026.11.1", build: "7", uptime: health?.uptime ?? ""))

    let routes = try client.routes()
    #expect(routes.count == 60)
    #expect(routes.contains { $0.name == "api.shop" && $0.upstream == "http://127.0.0.1:3001" })
}
