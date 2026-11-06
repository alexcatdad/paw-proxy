import Foundation

/// GET /health on the daemon's private socket.
public struct Health: Codable, Equatable, Sendable {
    public var status: String
    public var version: String
    public var build: String
    public var uptime: String

    public init(status: String, version: String, build: String = "", uptime: String) {
        self.status = status
        self.version = version
        self.build = build
        self.uptime = uptime
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        status = try c.decode(String.self, forKey: .status)
        version = try c.decode(String.self, forKey: .version)
        build = try c.decodeIfPresent(String.self, forKey: .build) ?? ""  // absent before build reporting
        uptime = try c.decodeIfPresent(String.self, forKey: .uptime) ?? ""
    }
}

/// One entry of GET /routes.
public struct Route: Codable, Equatable, Sendable {
    public var name: String
    public var upstream: String
    public var dir: String
    public var registered: Date
    public var lastHeartbeat: Date

    public init(name: String, upstream: String, dir: String, registered: Date, lastHeartbeat: Date) {
        self.name = name
        self.upstream = upstream
        self.dir = dir
        self.registered = registered
        self.lastHeartbeat = lastHeartbeat
    }
}

public enum DaemonJSON {
    /// Go encodes time.Time as RFC 3339 with optional fractional seconds.
    public static func decoder() -> JSONDecoder {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .custom { decoder in
            let s = try decoder.singleValueContainer().decode(String.self)
            if let date = parseRFC3339(s) { return date }
            throw DecodingError.dataCorrupted(
                .init(
                    codingPath: decoder.codingPath,
                    debugDescription: "bad RFC 3339 date \(s)"))
        }
        return d
    }

    static func parseRFC3339(_ s: String) -> Date? {
        let frac = ISO8601DateFormatter()
        frac.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = frac.date(from: s) { return d }
        let plain = ISO8601DateFormatter()
        plain.formatOptions = [.withInternetDateTime]
        if let d = plain.date(from: s) { return d }
        // ISO8601DateFormatter accepts at most millisecond precision on some
        // releases; Go emits nanoseconds. Trim to milliseconds and retry.
        if let dot = s.firstIndex(of: "."),
            let zone = s[dot...].firstIndex(where: { $0 == "Z" || $0 == "+" || $0 == "-" })
        {
            let digits = s[s.index(after: dot)..<zone]
            let trimmed =
                String(s[..<dot]) + "." + String(digits.prefix(3)).padding(toLength: 3, withPad: "0", startingAt: 0)
                + String(s[zone...])
            return frac.date(from: trimmed)
        }
        return nil
    }
}
