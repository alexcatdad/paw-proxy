import Foundation
import MenubarKit
import Testing
import UserNotifications

@testable import PawProxyNative

/// Records what would be posted, and whether with a sound.
private final class RecordingCenter: NotificationCenterClient, @unchecked Sendable {
    private let lock = NSLock()
    private var posted: [(String, Bool)] = []

    var deliveries: [(title: String, playSound: Bool)] { lock.withLock { posted.map { ($0.0, $0.1) } } }

    func requestAuthorization() async throws -> Bool { true }
    func authorizationStatus() async -> UNAuthorizationStatus { .authorized }
    func add(_ notification: MenubarNotification, playSound: Bool) async throws {
        lock.withLock { posted.append((notification.title, playSound)) }
    }
}

/// A throwaway defaults domain, removed after the test.
private func withSuite(_ body: (String, UserDefaults) async throws -> Void) async throws {
    let name = "paw-proxy.tests.\(UUID().uuidString)"
    let defaults = try #require(UserDefaults(suiteName: name))
    defer { defaults.removePersistentDomain(forName: name) }
    try await body(name, defaults)
}

private let event = RouteEvent(id: 1, kind: .live, routes: ["shop"], time: Date())

@Test func routeNotificationsAreSilentUnlessTheUserTurnsOnTheSound() async throws {
    try await withSuite { suite, defaults in
        let center = RecordingCenter()
        let notifier = RouteNotifications.notifier(center: center, suiteName: suite)

        defaults.set(true, forKey: NotificationPreferences.enabledKey)
        #expect(await notifier.deliver(RouteEventFeed.notification(for: event)) == .delivered)

        defaults.set(true, forKey: NotificationPreferences.soundEnabledKey)
        #expect(await notifier.deliver(RouteEventFeed.notification(for: event)) == .delivered)

        // Read on every delivery: turning the sound off again takes effect at once.
        defaults.set(false, forKey: NotificationPreferences.soundEnabledKey)
        #expect(await notifier.deliver(RouteEventFeed.notification(for: event)) == .delivered)

        #expect(center.deliveries.map(\.playSound) == [false, true, false])
        #expect(center.deliveries.allSatisfy { $0.title == "shop.test is live" })
    }
}

@Test func nothingIsPostedWhileNotificationsAreOffEvenWithTheSoundOn() async throws {
    try await withSuite { suite, defaults in
        let center = RecordingCenter()
        defaults.set(true, forKey: NotificationPreferences.soundEnabledKey)
        let notifier = RouteNotifications.notifier(center: center, suiteName: suite)
        #expect(await notifier.deliver(RouteEventFeed.notification(for: event)) == .disabled)
        #expect(center.deliveries.isEmpty)
    }
}

@Test func preferencesReadMissingKeysAsOff() async throws {
    try await withSuite { _, defaults in
        #expect(
            RouteNotifications.preferences(defaults) == NotificationPreferences(enabled: false, soundEnabled: false))
        defaults.set(true, forKey: NotificationPreferences.enabledKey)
        defaults.set(true, forKey: NotificationPreferences.soundEnabledKey)
        #expect(RouteNotifications.preferences(defaults) == NotificationPreferences(enabled: true, soundEnabled: true))
    }
}
