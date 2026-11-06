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

/// GET /notifications?after=<cursor>: events newer than `after`, the newest
/// ID the daemon has issued, and the daemon run (`epoch`) those IDs belong to.
/// Daemons before the epoch was added send none.
public struct RouteEventPage: Equatable, Sendable {
    public var epoch: String?
    public var cursor: UInt64
    public var events: [RouteEvent]

    public init(cursor: UInt64, events: [RouteEvent], epoch: String? = nil) {
        self.epoch = epoch
        self.cursor = cursor
        self.events = events
    }
}

extension RouteEventPage: Decodable {
    private enum CodingKeys: String, CodingKey { case epoch, cursor, notifications }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        epoch = try c.decodeIfPresent(String.self, forKey: .epoch)
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
/// - The first page primes the cursor. Of the events already queued it shows
///   only those from `since` on (when the app started listening), so `up`
///   runs between launch and the first poll are not lost, while the backlog
///   from before is not replayed.
/// - The cursor only moves forward, so a page is never shown twice.
/// - Daemon IDs start again at 1 on every daemon start. The page's `epoch`
///   names the daemon run; a new epoch means the cursor belongs to an old
///   daemon, so the feed asks again from 0 and shows the new daemon's events.
///   Without an epoch (an older daemon), a cursor that went backwards means
///   the same thing.
/// - Notification identifiers are stable per set of routes, so "stopped"
///   replaces that project's "is live" banner and a project restarted in a
///   loop keeps one notification instead of stacking. Within one page only
///   the latest event per set of routes is shown.
public struct RouteEventFeed: Equatable, Sendable {
    public private(set) var cursor: UInt64?
    public private(set) var epoch: String?
    /// When the app started listening; nil shows no queued events at all.
    public private(set) var since: Date?

    public init(cursor: UInt64? = nil, epoch: String? = nil, since: Date? = nil) {
        self.cursor = cursor
        self.epoch = epoch
        self.since = since
    }

    /// The cursor to send as `after`. Unprimed feeds ask for everything.
    public var after: UInt64 { cursor ?? 0 }

    /// Forget the cursor, for example when notifications are turned off, so
    /// turning them back on does not replay what happened meanwhile. Events
    /// from `since` on still show once the feed primes again.
    public mutating func reset(since: Date? = nil) {
        cursor = nil
        epoch = nil
        self.since = since
    }

    public mutating func accept(_ page: RouteEventPage, tld: String = "test") -> [MenubarNotification] {
        guard let current = cursor else {
            cursor = page.cursor
            epoch = page.epoch
            guard let since else { return [] }
            return Self.notifications(page.events.filter { $0.time >= since }, tld: tld)
        }
        let newDaemon = page.epoch.map { $0 != epoch } ?? (page.cursor < current)
        if newDaemon {
            // This page was filtered by the old daemon's cursor, so ask
            // again from the start next time.
            cursor = 0
            epoch = page.epoch
            return []
        }
        cursor = page.cursor
        return Self.notifications(page.events.filter { $0.id > current }, tld: tld)
    }

    /// The latest event per set of routes, oldest first.
    private static func notifications(_ events: [RouteEvent], tld: String) -> [MenubarNotification] {
        var seen = Set<String>()
        let latestFirst =
            events
            .filter { !$0.routes.isEmpty }
            .sorted { $0.id > $1.id }
            .map { notification(for: $0, tld: tld) }
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
