import Foundation
import MenubarKit

/// One event from GET /notifications. `up` reports route names only; the
/// app writes all display text, so nothing a client sends is shown verbatim.
public struct RouteEvent: Codable, Equatable, Sendable {
    public enum Kind: String, Codable, Sendable {
        case live = "routes.live"
        case removed = "routes.removed"
    }

    public var id: UInt64
    public var kind: Kind
    public var routes: [String]
    public var time: Date

    public init(id: UInt64, kind: Kind, routes: [String], time: Date) {
        self.id = id
        self.kind = kind
        self.routes = routes
        self.time = time
    }
}

/// GET /notifications?after=<cursor>: events newer than `after`, and the
/// newest ID the daemon has issued.
public struct RouteEventPage: Equatable, Sendable {
    public var cursor: UInt64
    public var events: [RouteEvent]

    public init(cursor: UInt64, events: [RouteEvent]) {
        self.cursor = cursor
        self.events = events
    }
}

extension RouteEventPage: Decodable {
    private enum CodingKeys: String, CodingKey { case cursor, notifications }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        cursor = try c.decode(UInt64.self, forKey: .cursor)
        // Unknown kinds (a newer daemon) are skipped, not fatal.
        let raw = try c.decodeIfPresent([Lossy].self, forKey: .notifications) ?? []
        events = raw.compactMap(\.event)
    }

    private struct Lossy: Decodable {
        var event: RouteEvent?
        init(from decoder: Decoder) throws { event = try? RouteEvent(from: decoder) }
    }
}

extension DaemonClient {
    public func routeEvents(after cursor: UInt64) throws -> RouteEventPage {
        try DaemonJSON.decoder().decode(RouteEventPage.self, from: get("/notifications?after=\(cursor)"))
    }
}

/// Turns polled daemon pages into notifications, each event at most once.
///
/// - The first page only primes the cursor: events from before the app
///   started (or before the user turned notifications on) are not replayed.
/// - The cursor only moves forward, so a page is never shown twice. When the
///   daemon restarts its IDs start again at 1; a cursor that went backwards
///   means a new daemon, so the feed resets and shows its events.
/// - Notification identifiers are stable per set of routes, so "stopped"
///   replaces that project's "is live" banner and a project restarted in a
///   loop keeps one notification instead of stacking. Within one page only
///   the latest event per set of routes is shown.
public struct RouteEventFeed: Equatable, Sendable {
    public private(set) var cursor: UInt64?

    public init(cursor: UInt64? = nil) {
        self.cursor = cursor
    }

    /// The cursor to send as `after`. Unprimed feeds ask for everything.
    public var after: UInt64 { cursor ?? 0 }

    /// Forget the cursor, for example when notifications are turned off, so
    /// turning them back on does not replay what happened meanwhile.
    public mutating func reset() {
        cursor = nil
    }

    public mutating func accept(_ page: RouteEventPage, tld: String = "test") -> [MenubarNotification] {
        guard let current = cursor else {
            cursor = page.cursor
            return []
        }
        if page.cursor < current {
            // The daemon restarted. This page was filtered by the old
            // cursor, so ask again from the start next time.
            cursor = 0
            return []
        }
        cursor = page.cursor
        var seen = Set<String>()
        let latestFirst = page.events
            .filter { $0.id > current && !$0.routes.isEmpty }
            .sorted { $0.id > $1.id }
            .map { Self.notification(for: $0, tld: tld) }
            .filter { seen.insert($0.identifier).inserted }
        return latestFirst.reversed()
    }

    public static func notification(for event: RouteEvent, tld: String = "test") -> MenubarNotification {
        let hosts = event.routes.map { "\($0).\(tld)" }
        let identifier = "paw-proxy.routes.\(event.routes.sorted().joined(separator: ","))"
        let one = hosts.count == 1
        switch event.kind {
        case .live:
            let url = "https://\(hosts[0])"
            return MenubarNotification(
                title: one ? "\(hosts[0]) is live" : "\(hosts.count) projects are live",
                body: one ? url : Self.list(hosts),
                threadIdentifier: event.kind.rawValue, identifier: identifier, userInfo: ["url": url])
        case .removed:
            return MenubarNotification(
                title: one ? "\(hosts[0]) stopped" : "\(hosts.count) projects stopped",
                body: one ? "Its route was removed." : Self.list(hosts),
                threadIdentifier: event.kind.rawValue, identifier: identifier)
        }
    }

    /// The page a clicked notification opens: only an https URL on the
    /// proxy's own TLD, whatever else the userInfo carries.
    public static func clickURL(_ userInfo: [String: String], tld: String = "test") -> URL? {
        guard let raw = userInfo["url"], let url = URL(string: raw), url.scheme == "https",
            let host = url.host, host.hasSuffix(".\(tld)")
        else { return nil }
        return url
    }

    /// "a.test, b.test and 3 more"
    static func list(_ hosts: [String], shown: Int = 3) -> String {
        guard hosts.count > shown else {
            return hosts.dropLast().joined(separator: ", ") + " and " + hosts[hosts.count - 1]
        }
        return hosts.prefix(shown).joined(separator: ", ") + " and \(hosts.count - shown) more"
    }
}
