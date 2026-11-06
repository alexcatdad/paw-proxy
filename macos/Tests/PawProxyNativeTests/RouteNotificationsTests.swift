import Foundation
import MenubarKit
import Testing

@testable import PawProxyNative

private let t0 = Date(timeIntervalSince1970: 1_800_000_000)

private func live(_ id: UInt64, _ routes: String...) -> RouteEvent {
    RouteEvent(id: id, kind: .live, routes: routes, time: t0)
}

private func removed(_ id: UInt64, _ routes: String...) -> RouteEvent {
    RouteEvent(id: id, kind: .removed, routes: routes, time: t0)
}

private func page(_ cursor: UInt64, _ events: RouteEvent...) -> RouteEventPage {
    RouteEventPage(cursor: cursor, events: events)
}

private func live(_ id: UInt64, at time: Date, _ routes: String...) -> RouteEvent {
    RouteEvent(id: id, kind: .live, routes: routes, time: time)
}

private func page(epoch: String, _ cursor: UInt64, _ events: RouteEvent...) -> RouteEventPage {
    RouteEventPage(cursor: cursor, events: events, epoch: epoch)
}

@Test func firstPageShowsEventsFromWhenTheAppStartedListening() {
    var feed = RouteEventFeed(since: t0)
    let backlog = live(1, at: t0.addingTimeInterval(-60), "old")
    let fresh = live(2, at: t0.addingTimeInterval(1), "new")
    #expect(feed.accept(page(epoch: "a", 2, backlog, fresh)).map(\.title) == ["new.test is live"])
    #expect(feed.cursor == 2)
    #expect(feed.epoch == "a")
    // Primed: the same events are not shown again.
    #expect(feed.accept(page(epoch: "a", 2, backlog, fresh)).isEmpty)
}

@Test func newEpochResetsTheCursorEvenWhenIDsCaughtUp() {
    var feed = RouteEventFeed(cursor: 3, epoch: "a", since: t0)
    // A restarted daemon has already issued 5 IDs; the cursor did not go
    // backwards, but the page was filtered by the old cursor (after=3).
    #expect(feed.accept(page(epoch: "b", 5, live(4, "d"), live(5, "e"))).isEmpty)
    #expect(feed.after == 0)
    #expect(feed.epoch == "b")
    let notes = feed.accept(page(epoch: "b", 5, live(1, "a"), live(2, "b"), live(3, "c"), live(4, "d"), live(5, "e")))
    #expect(
        notes.map(\.title) == [
            "a.test is live", "b.test is live", "c.test is live", "d.test is live", "e.test is live",
        ])
    #expect(feed.cursor == 5)
}

@Test func sameEpochKeepsTheCursor() {
    var feed = RouteEventFeed(cursor: 3, epoch: "a")
    #expect(feed.accept(page(epoch: "a", 4, live(4, "shop"))).map(\.title) == ["shop.test is live"])
    #expect(feed.cursor == 4)
}

@Test func resetKeepsTheNewSinceAndForgetsTheEpoch() {
    var feed = RouteEventFeed(cursor: 9, epoch: "a", since: t0)
    let later = t0.addingTimeInterval(100)
    feed.reset(since: later)
    #expect(feed.cursor == nil && feed.epoch == nil && feed.since == later)
    let notes = feed.accept(
        page(epoch: "a", 11, live(10, at: t0.addingTimeInterval(50), "x"), live(11, at: later, "y")))
    #expect(notes.map(\.title) == ["y.test is live"])
}

@Test func firstPageOnlyPrimesTheCursor() {
    var feed = RouteEventFeed()
    #expect(feed.after == 0)
    #expect(feed.accept(page(2, live(1, "old"), live(2, "older"))).isEmpty)
    #expect(feed.cursor == 2)
    #expect(feed.after == 2)
}

@Test func newEventsPostOnceAndTheCursorAdvances() {
    var feed = RouteEventFeed(cursor: 2)
    let notes = feed.accept(page(3, live(3, "shop")))
    #expect(notes.map(\.title) == ["shop.test is live"])
    #expect(feed.cursor == 3)
    // The same page again (a retried poll) posts nothing.
    #expect(feed.accept(page(3, live(3, "shop"))).isEmpty)
    // An empty page keeps the cursor.
    #expect(feed.accept(page(3)).isEmpty)
    #expect(feed.cursor == 3)
}

@Test func eventsAtOrBelowTheCursorAreIgnored() {
    var feed = RouteEventFeed(cursor: 5)
    #expect(feed.accept(page(6, live(4, "a"), live(5, "b"), live(6, "c"))).map(\.title) == ["c.test is live"])
}

@Test func daemonRestartResetsTheCursor() {
    var feed = RouteEventFeed(cursor: 40)
    // A new daemon issued IDs 1...2; the page was filtered by after=40.
    #expect(feed.accept(page(2)).isEmpty)
    #expect(feed.after == 0)
    #expect(feed.accept(page(2, live(1, "a"), live(2, "b"))).map(\.title) == ["a.test is live", "b.test is live"])
    #expect(feed.cursor == 2)
}

@Test func latestEventPerProjectWinsWithinAPage() {
    var feed = RouteEventFeed(cursor: 0)
    let notes = feed.accept(page(4, live(1, "shop"), removed(2, "shop"), live(3, "shop"), live(4, "blog")))
    #expect(notes.map(\.title) == ["shop.test is live", "blog.test is live"])
    #expect(Set(notes.map(\.identifier)).count == 2)
}

@Test func stoppedReplacesTheLiveNotification() {
    let up = RouteEventFeed.notification(for: live(1, "shop"))
    let down = RouteEventFeed.notification(for: removed(2, "shop"))
    #expect(up.identifier == down.identifier)
    #expect(down.title == "shop.test stopped")
    #expect(down.userInfo.isEmpty)
    // Route order does not change the identity.
    #expect(
        RouteEventFeed.notification(for: live(1, "b", "a")).identifier
            == RouteEventFeed.notification(for: live(2, "a", "b")).identifier)
}

@Test func liveNotificationWording() {
    let one = RouteEventFeed.notification(for: live(1, "shop"))
    #expect(
        one
            == MenubarNotification(
                title: "shop.test is live", body: "https://shop.test",
                threadIdentifier: "routes.live", identifier: "paw-proxy.routes.shop",
                userInfo: ["url": "https://shop.test"]))
    let many = RouteEventFeed.notification(for: live(1, "web", "api", "db", "cache", "queue"))
    #expect(many.title == "5 projects are live")
    #expect(many.body == "web.test, api.test, db.test and 2 more")
    #expect(many.userInfo["url"] == "https://web.test")
    #expect(RouteEventFeed.notification(for: removed(1, "web", "api")).body == "web.test and api.test")
}

@Test func resetForgetsTheCursor() {
    var feed = RouteEventFeed(cursor: 9)
    feed.reset()
    #expect(feed.cursor == nil)
    #expect(feed.accept(page(12, live(12, "x"))).isEmpty)
}

@Test func decodesTheDaemonPageAndSkipsUnknownKinds() throws {
    let json = """
        {"cursor":3,"notifications":[
          {"id":1,"kind":"routes.live","routes":["shop","api.shop"],"time":"2026-11-05T10:00:00.123456789+02:00"},
          {"id":2,"kind":"routes.renamed","routes":["x"],"time":"2026-11-05T10:00:01Z"},
          {"id":3,"kind":"routes.removed","routes":["shop"],"time":"2026-11-05T10:00:02Z"}]}
        """
    let page = try DaemonJSON.decoder().decode(RouteEventPage.self, from: Data(json.utf8))
    #expect(page.cursor == 3)
    #expect(page.events.map(\.id) == [1, 3])
    #expect(page.events[0].routes == ["shop", "api.shop"])
    #expect(page.events[1].kind == .removed)

    #expect(page.epoch == nil)

    let empty = try DaemonJSON.decoder().decode(
        RouteEventPage.self, from: Data(#"{"cursor":0,"notifications":[]}"#.utf8))
    #expect(empty == RouteEventPage(cursor: 0, events: []))

    let withEpoch = try DaemonJSON.decoder().decode(
        RouteEventPage.self, from: Data(#"{"epoch":"9f2c","cursor":0,"notifications":[]}"#.utf8))
    #expect(withEpoch == RouteEventPage(cursor: 0, events: [], epoch: "9f2c"))
}

@Test func clicksOpenOnlyProxyURLs() {
    #expect(RouteEventFeed.clickURL(["url": "https://shop.test"]) == URL(string: "https://shop.test"))
    #expect(RouteEventFeed.clickURL(["url": "http://shop.test"]) == nil)
    #expect(RouteEventFeed.clickURL(["url": "https://example.com"]) == nil)
    #expect(RouteEventFeed.clickURL(["url": "file:///etc/passwd"]) == nil)
    #expect(RouteEventFeed.clickURL([:]) == nil)
}
