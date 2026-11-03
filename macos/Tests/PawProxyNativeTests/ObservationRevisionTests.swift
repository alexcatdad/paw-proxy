import Testing
@testable import PawProxyNative

@Test func staleRepliesAreDropped() {
    var rev = ObservationRevision()
    let refresh = rev.current      // refresh starts
    #expect(rev.accepts(refresh))
    rev.bump()                      // user clicks Restart mid-flight
    #expect(!rev.accepts(refresh))  // the old reply must not overwrite "Restarting…"
    let next = rev.current
    #expect(rev.accepts(next))
}
