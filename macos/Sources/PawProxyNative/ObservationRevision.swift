/// Drops stale asynchronous status replies.
///
/// Every user action (restart, turn on/off) bumps the revision; a refresh
/// captures the revision when it starts and its result is applied only if
/// no action happened in between. This keeps a reply from before a restart
/// from overwriting the "Restarting…" state.
public struct ObservationRevision: Sendable {
    public private(set) var current = 0
    public init() {}

    @discardableResult
    public mutating func bump() -> Int {
        current += 1
        return current
    }

    public func accepts(_ revision: Int) -> Bool { revision == current }
}
