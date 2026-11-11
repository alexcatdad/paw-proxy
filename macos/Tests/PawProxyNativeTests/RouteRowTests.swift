import Foundation
import Testing

@testable import PawProxyNative

private let now = Date(timeIntervalSince1970: 1_000_000)
private func route(_ name: String, age: TimeInterval, dir: String = "/Users/dev/src/shop") -> Route {
    Route(
        name: name, upstream: "http://127.0.0.1:3000", dir: dir,
        registered: now.addingTimeInterval(-600), lastHeartbeat: now.addingTimeInterval(-age))
}

@Test func formatsRow() {
    let row = RouteRow(route: route("shop", age: 3), now: now)
    #expect(row.host == "shop.test")
    #expect(row.target == "127.0.0.1:3000")
    #expect(row.project == "shop")
    #expect(row.heartbeatAge == "3s ago")
    #expect(!row.isStale)
    #expect(row.url?.absoluteString == "https://shop.test")
}

@Test func staleAfterHalfTheTimeout() {
    #expect(!RouteRow(route: route("a", age: 15), now: now, timeout: 30).isStale)
    #expect(RouteRow(route: route("a", age: 16), now: now, timeout: 30).isStale)
}

@Test func ageFormatting() {
    #expect(RouteRow.formatAge(0) == "now")
    #expect(RouteRow.formatAge(-5) == "now")
    #expect(RouteRow.formatAge(59) == "59s ago")
    #expect(RouteRow.formatAge(125) == "2m ago")
    #expect(RouteRow.formatAge(7300) == "2h ago")
}

@Test func emptyDirAndSorting() {
    let rows = RouteRow.rows(
        [route("web", age: 1, dir: ""), route("api.shop", age: 1), route("Admin", age: 1)], now: now)
    #expect(rows.map(\.host) == ["Admin.test", "api.shop.test", "web.test"])
    #expect(rows[2].project == "")
}

@Test func tailnetRouteShowsInSubtitle() {
    var served = route("shop", age: 1)
    served.tailnetURL = "https://mac.example.ts.net:20001"
    let row = RouteRow(route: served, now: now)
    #expect(row.tailnetURL?.absoluteString == "https://mac.example.ts.net:20001")
    #expect(row.subtitle == "→ 127.0.0.1:3000 · shop · tailnet")
    #expect(RouteRow(route: route("shop", age: 1), now: now).subtitle == "→ 127.0.0.1:3000 · shop")
    #expect(RouteRow(route: route("web", age: 1, dir: ""), now: now).subtitle == "→ 127.0.0.1:3000")
}
