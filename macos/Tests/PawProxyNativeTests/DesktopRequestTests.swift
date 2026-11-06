import Foundation
import Testing

@testable import PawProxyNative

/// internal/desktop/testdata/vectors.json, shared with the Go test suite.
private struct Vectors: Decodable {
    struct Case: Decodable { var name: String; var json: String; var valid: Bool }
    var id: String
    var requests: [Case]
    var replies: [Case]

    static func load() throws -> Vectors {
        let url = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().appendingPathComponent("internal/desktop/testdata/vectors.json")
        return try JSONDecoder().decode(Vectors.self, from: Data(contentsOf: url))
    }
}

@Test func requestVectors() throws {
    let v = try Vectors.load()
    for c in v.requests {
        let parsed = try? Desktop.parseRequest(Data(c.json.utf8))
        #expect((parsed != nil) == c.valid, "request vector \(c.name)")
    }
}

@Test func replyVectors() throws {
    let v = try Vectors.load()
    for c in v.replies {
        let parsed = try? Desktop.parseReply(Data(c.json.utf8), id: v.id)
        #expect((parsed != nil) == c.valid, "reply vector \(c.name)")
        // Everything the app writes must also be accepted by the CLI: a valid
        // vector re-encoded by Swift parses back to the same reply.
        if let parsed {
            let data = try JSONEncoder().encode(
                Desktop.Reply(id: parsed.id, outcome: parsed.outcome, detail: parsed.detail))
            #expect(try Desktop.parseReply(data, id: v.id) == parsed)
        }
    }
}

/// A temp support dir with a requests/ folder.
private struct RequestDir {
    let paths: Paths
    let dir: URL
    let id = "0123456789abcdef0123456789abcdef"

    init() throws {
        let home = FileManager.default.temporaryDirectory.appendingPathComponent("pp-req-\(UUID().uuidString)")
        paths = Paths(home: home, resolverDir: home)
        dir = paths.supportDir.appendingPathComponent("requests")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    }

    @discardableResult
    func write(_ json: String, name: String? = nil, mode: Int = 0o600) throws -> URL {
        let url = dir.appendingPathComponent(name ?? "\(id).json")
        try Data(json.utf8).write(to: url)
        try FileManager.default.setAttributes([.posixPermissions: mode], ofItemAtPath: url.path)
        return url
    }

    func request(_ op: String) -> String { #"{"version":1,"id":"\#(id)","op":"\#(op)"}"# }

    func reply() throws -> Desktop.Reply {
        let url = dir.appendingPathComponent("\(id).reply.json")
        let mode = try FileManager.default.attributesOfItem(atPath: url.path)[.posixPermissions] as? Int
        #expect(mode == 0o600)
        return try Desktop.parseReply(Data(contentsOf: url), id: id)
    }

    var replyExists: Bool {
        FileManager.default.fileExists(atPath: dir.appendingPathComponent("\(id).reply.json").path)
    }
}

@Test func readsValidRequest() throws {
    let r = try RequestDir()
    let url = try r.write(r.request("register"))
    let env = try Desktop.readRequest(at: url, requestsDir: r.dir)
    #expect(env.request.op == .register)
    #expect(env.replyURL.lastPathComponent == "\(r.id).reply.json")
}

@Test func rejectsUnsafeRequestFiles() throws {
    let r = try RequestDir()
    // World-readable.
    var url = try r.write(r.request("register"), mode: 0o644)
    #expect(throws: Desktop.Invalid.self) { try Desktop.readRequest(at: url, requestsDir: r.dir) }
    // Wrong owner.
    url = try r.write(r.request("register"))
    #expect(throws: Desktop.Invalid.self) { try Desktop.readRequest(at: url, requestsDir: r.dir, owner: getuid() + 1) }
    // ID does not match the file name.
    url = try r.write(r.request("register"), name: "ffffffffffffffffffffffffffffffff.json")
    #expect(throws: Desktop.Invalid.self) { try Desktop.readRequest(at: url, requestsDir: r.dir) }
    // Oversized.
    url = try r.write(String(repeating: " ", count: Desktop.maxFileSize + 1))
    #expect(throws: Desktop.Invalid.self) { try Desktop.readRequest(at: url, requestsDir: r.dir) }
    // Symlink to an otherwise valid file.
    let target = try r.write(r.request("register"), name: "target")
    let link = r.dir.appendingPathComponent("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.json")
    try FileManager.default.createSymbolicLink(at: link, withDestinationURL: target)
    #expect(throws: Desktop.Invalid.self) { try Desktop.readRequest(at: link, requestsDir: r.dir) }
}

@Test func rejectsRequestsOutsideTheRequestsDir() throws {
    let r = try RequestDir()
    let outside = r.paths.supportDir.appendingPathComponent("\(r.id).json")
    try Data(r.request("register").utf8).write(to: outside)
    try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: outside.path)
    #expect(Desktop.replyURL(forRequest: outside, requestsDir: r.dir) == nil)
    #expect(throws: Desktop.Invalid.self) { try Desktop.readRequest(at: outside, requestsDir: r.dir) }
    // A reply path is never treated as a request.
    #expect(Desktop.replyURL(forRequest: r.dir.appendingPathComponent("\(r.id).reply.json"), requestsDir: r.dir) == nil)
}

private func agent(for r: RequestDir) -> (DaemonAgent, FakeRegistrar) {
    let registrar = FakeRegistrar()
    return (DaemonAgent(paths: r.paths, registrar: registrar, launchctl: FakeLaunchctl()), registrar)
}

@Test func handlerRepliesToStatus() throws {
    let r = try RequestDir()
    let url = try r.write(r.request("status"))
    #expect(RequestHandler.handle(requestPath: url.path, paths: r.paths, agent: agent(for: r).0, loginItem: nil) == 0)
    #expect(try r.reply() == Desktop.Reply(id: r.id, outcome: .notRegistered))
}

@Test func handlerRegistersAndUnregisters() throws {
    let r = try RequestDir()
    let (a, _) = agent(for: r)
    let item = FakeLoginItem()
    item.enabled = true

    var url = try r.write(r.request("register"))
    #expect(RequestHandler.handle(requestPath: url.path, paths: r.paths, agent: a, loginItem: item) == 0)
    #expect(try r.reply().outcome == .registered)

    try FileManager.default.removeItem(at: url)
    try FileManager.default.removeItem(at: r.dir.appendingPathComponent("\(r.id).reply.json"))
    url = try r.write(r.request("unregister"))
    #expect(RequestHandler.handle(requestPath: url.path, paths: r.paths, agent: a, loginItem: item) == 0)
    #expect(try r.reply().outcome == .unregistered)
    #expect(!item.enabled)  // the menu-bar login item goes too
}

@Test func handlerReportsInvalidRequestsAsFailed() throws {
    let r = try RequestDir()
    let (a, registrar) = agent(for: r)
    let url = try r.write(#"{"version":2,"id":"\#(r.id)","op":"register"}"#)
    #expect(RequestHandler.handle(requestPath: url.path, paths: r.paths, agent: a, loginItem: nil) == 0)
    #expect(try r.reply().outcome == .failed)
    #expect(registrar.log.isEmpty)
}

@Test func handlerWritesNothingForUntrustedPaths() throws {
    let r = try RequestDir()
    #expect(
        RequestHandler.handle(
            requestPath: "/tmp/\(r.id).json", paths: r.paths,
            agent: agent(for: r).0, loginItem: nil) == 2)
    #expect(!r.replyExists)
}

final class FakeLoginItem: LoginItem, @unchecked Sendable {
    var enabled = false
    var isEnabled: Bool { enabled }
    func setEnabled(_ enabled: Bool) throws { self.enabled = enabled }
}
