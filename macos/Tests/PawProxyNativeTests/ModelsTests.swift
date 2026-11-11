import Foundation
import Testing

@testable import PawProxyNative

@Test func decodesGoHealthWithAndWithoutBuild() throws {
    let withBuild = Data(#"{"build":"7","status":"ok","uptime":"1m2s","version":"2026.11.1"}"#.utf8)
    #expect(
        try DaemonJSON.decoder().decode(Health.self, from: withBuild)
            == Health(status: "ok", version: "2026.11.1", build: "7", uptime: "1m2s"))
    let old = Data(#"{"status":"ok","uptime":"5s","version":"1.12.3"}"#.utf8)
    #expect(try DaemonJSON.decoder().decode(Health.self, from: old).build == "")
}

@Test func decodesGoRoutesWithNanosecondTimes() throws {
    let json = Data(
        #"[{"name":"shop","upstream":"http://127.0.0.1:3000","dir":"/src/shop","registered":"2026-11-03T21:00:00.123456789+02:00","lastHeartbeat":"2026-11-03T19:00:05Z"}]"#
            .utf8)
    let routes = try DaemonJSON.decoder().decode([Route].self, from: json)
    #expect(routes.count == 1)
    #expect(routes[0].registered.timeIntervalSince1970 == 1_793_732_400.123)
    #expect(routes[0].lastHeartbeat.timeIntervalSince(routes[0].registered) > 4.8)
}

@Test func rejectsGarbageDates() {
    let json = Data(
        #"[{"name":"a","upstream":"u","dir":"","registered":"yesterday","lastHeartbeat":"2026-11-03T19:00:05Z"}]"#.utf8)
    #expect(throws: DecodingError.self) { try DaemonJSON.decoder().decode([Route].self, from: json) }
}

@Test func identityDevDetection() {
    #expect(Identity.isDevelopment(bundleID: "com.alexcatdad.paw-proxy.dev"))
    #expect(!Identity.isDevelopment(bundleID: Identity.bundleID))
    #expect(!Identity.isDevelopment(bundleID: nil))
}

@Test func pathsMirrorTheGoDaemon() {
    let p = Paths(home: URL(fileURLWithPath: "/Users/u"), resolverDir: URL(fileURLWithPath: "/etc/resolver"))
    #expect(p.socket.path == "/Users/u/Library/Application Support/paw-proxy/paw-proxy.sock")
    #expect(p.log.path == "/Users/u/Library/Logs/paw-proxy.log")
    #expect(p.resolver().path == "/etc/resolver/test")
}

@Test func decodesTailnetURLWhenPresent() throws {
    let json = Data(
        #"[{"name":"shop","upstream":"localhost:3000","dir":"/src/shop","registered":"2026-11-03T19:00:00Z","lastHeartbeat":"2026-11-03T19:00:05Z","tailnetURL":"https://mac.example.ts.net:20001"}]"#
            .utf8)
    let routes = try DaemonJSON.decoder().decode([Route].self, from: json)
    #expect(routes[0].tailnetURL == "https://mac.example.ts.net:20001")
}
