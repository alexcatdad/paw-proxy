import Foundation

/// Display model for one route in the menu.
public struct RouteRow: Equatable, Identifiable, Sendable {
    public var id: String { name }
    public var name: String
    public var host: String
    public var target: String
    public var project: String
    public var heartbeatAge: String
    public var isStale: Bool
    public var url: URL?
    public var tailnetURL: URL?

    /// Second line of the row: target, project folder, and whether the route
    /// is also on the tailnet.
    public var subtitle: String {
        (["→ \(target)", project] + (tailnetURL == nil ? [] : ["tailnet"]))
            .filter { !$0.isEmpty }.joined(separator: " · ")
    }

    /// - Parameter timeout: the daemon's heartbeat timeout; rows older than
    ///   half of it are shown as stale (the route is about to expire).
    public init(route: Route, now: Date, tld: String = "test", timeout: TimeInterval = 30) {
        name = route.name
        host = "\(route.name).\(tld)"
        target = Self.trimScheme(route.upstream)
        project = route.dir.isEmpty ? "" : URL(fileURLWithPath: route.dir).lastPathComponent
        let age = max(0, now.timeIntervalSince(route.lastHeartbeat))
        heartbeatAge = Self.formatAge(age)
        isStale = age > timeout / 2
        url = URL(string: "https://\(host)")
        tailnetURL = route.tailnetURL.flatMap(URL.init(string:))
    }

    public static func rows(_ routes: [Route], now: Date, tld: String = "test", timeout: TimeInterval = 30)
        -> [RouteRow]
    {
        routes.map { RouteRow(route: $0, now: now, tld: tld, timeout: timeout) }
            .sorted { $0.host.localizedStandardCompare($1.host) == .orderedAscending }
    }

    static func trimScheme(_ upstream: String) -> String {
        for prefix in ["http://", "https://"] where upstream.hasPrefix(prefix) {
            return String(upstream.dropFirst(prefix.count))
        }
        return upstream
    }

    static func formatAge(_ seconds: TimeInterval) -> String {
        switch seconds {
        case ..<2: return "now"
        case ..<60: return "\(Int(seconds))s ago"
        case ..<3600: return "\(Int(seconds / 60))m ago"
        default: return "\(Int(seconds / 3600))h ago"
        }
    }
}
