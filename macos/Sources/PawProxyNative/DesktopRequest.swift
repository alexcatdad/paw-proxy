import Foundation

/// The app side of the one-shot request/reply exchange with the CLI
/// (internal/desktop). The CLI writes `<requests>/<id>.json` (0600) and
/// launches a fresh app instance with `--request <path>`; the app validates
/// the file, performs the operation and writes `<id>.reply.json` (0600) next
/// to it. Both sides read internal/desktop/testdata/vectors.json in tests.
public enum Desktop {
    public static let protocolVersion = 1
    public static let maxFileSize = 64 << 10

    public enum Op: String, Codable, Sendable { case register, unregister, status }

    public enum Outcome: String, Codable, Sendable {
        case registered, requiresApproval, unregistered, notRegistered, rolledBack, failed
    }

    public struct Request: Codable, Equatable, Sendable {
        public var version: Int
        public var id: String
        public var op: Op
    }

    public struct Reply: Codable, Equatable, Sendable {
        public var version: Int
        public var id: String
        public var outcome: Outcome
        public var detail: String?

        public init(id: String, outcome: Outcome, detail: String? = nil) {
            self.version = Desktop.protocolVersion
            self.id = id
            self.outcome = outcome
            self.detail = detail?.isEmpty == true ? nil : detail
        }
    }

    public struct Invalid: Error, CustomStringConvertible, Equatable {
        public var description: String
        init(_ d: String) { description = d }
    }

    static func isValidID(_ id: String) -> Bool {
        id.utf8.count == 32 && id.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
    }

    /// Validates a request payload (format only; file checks are separate).
    public static func parseRequest(_ data: Data) throws -> Request {
        guard data.count <= maxFileSize else { throw Invalid("request too large") }
        let r: Request
        do { r = try JSONDecoder().decode(Request.self, from: data) } catch {
            throw Invalid("malformed request")
        }
        guard r.version == protocolVersion else { throw Invalid("unsupported request version \(r.version)") }
        guard isValidID(r.id) else { throw Invalid("invalid request id") }
        return r
    }

    /// Validates a reply payload for request `id`. The CLI does the same in Go;
    /// the Swift copy keeps both sides pinned to the shared vectors.
    public static func parseReply(_ data: Data, id: String) throws -> Reply {
        guard data.count <= maxFileSize else { throw Invalid("reply too large") }
        let r: Reply
        do { r = try JSONDecoder().decode(Reply.self, from: data) } catch {
            throw Invalid("malformed reply")
        }
        guard r.version == protocolVersion else { throw Invalid("unsupported reply version \(r.version)") }
        guard isValidID(r.id), r.id == id else { throw Invalid("reply does not match the request") }
        return r
    }

    /// A request file accepted for handling, and where its reply goes.
    public struct Envelope: Equatable, Sendable {
        public var request: Request
        public var replyURL: URL
    }

    /// The reply location for a request path, if the path is a well-formed
    /// `<id>.json` inside `requestsDir`. Used both to accept a request and to
    /// report a failure for one that fails later checks.
    public static func replyURL(forRequest path: URL, requestsDir: URL) -> (id: String, url: URL)? {
        let dir = path.deletingLastPathComponent().standardizedFileURL.resolvingSymlinksInPath()
        guard dir.path == requestsDir.standardizedFileURL.resolvingSymlinksInPath().path else { return nil }
        let name = path.lastPathComponent
        guard name.hasSuffix(".json"), !name.hasSuffix(".reply.json") else { return nil }
        let id = String(name.dropLast(5))
        guard isValidID(id) else { return nil }
        return (id, dir.appendingPathComponent("\(id).reply.json"))
    }

    /// Reads and validates a request file: inside `requestsDir`, named
    /// `<id>.json`, a regular file (not a symlink) owned by `owner` with mode
    /// 0600, at most 64 KiB, and answering its own file name.
    public static func readRequest(at path: URL, requestsDir: URL, owner: uid_t = getuid()) throws -> Envelope {
        guard let (id, replyURL) = replyURL(forRequest: path, requestsDir: requestsDir) else {
            throw Invalid("request path is not a request file in \(requestsDir.path)")
        }
        let fileURL = replyURL.deletingLastPathComponent().appendingPathComponent("\(id).json")
        var st = stat()
        guard lstat(fileURL.path, &st) == 0 else { throw Invalid("request file missing") }
        guard (st.st_mode & S_IFMT) == S_IFREG else { throw Invalid("request is not a regular file") }
        guard st.st_uid == owner else { throw Invalid("request is not owned by the current user") }
        guard (st.st_mode & 0o777) == 0o600 else { throw Invalid("request file mode must be 0600") }
        guard st.st_size <= maxFileSize else { throw Invalid("request too large") }

        let data = try Data(contentsOf: fileURL)
        let request = try parseRequest(data)
        guard request.id == id else { throw Invalid("request id does not match its file name") }
        return Envelope(request: request, replyURL: replyURL)
    }

    /// Writes a reply atomically with mode 0600 (temp file, then rename), so
    /// the CLI never reads a partial file.
    public static func writeReply(_ reply: Reply, to url: URL) throws {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let data = try encoder.encode(reply)
        let tmp = url.deletingLastPathComponent().appendingPathComponent(".\(reply.id).reply.tmp")
        unlink(tmp.path)
        let fd = open(tmp.path, O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW, 0o600)
        guard fd >= 0 else { throw Invalid("cannot create reply: \(String(cString: strerror(errno)))") }
        let ok = data.withUnsafeBytes { buf in write(fd, buf.baseAddress, buf.count) == buf.count }
        close(fd)
        guard ok, rename(tmp.path, url.path) == 0 else {
            unlink(tmp.path)
            throw Invalid("cannot write reply: \(String(cString: strerror(errno)))")
        }
    }
}
