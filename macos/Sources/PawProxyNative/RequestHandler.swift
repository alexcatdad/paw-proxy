import Foundation

/// Handles one `--request <path>` launch: validate, act, reply.
public enum RequestHandler {
    /// Exit status for the request instance: 0 when a reply was written.
    public static func handle(requestPath: String, paths: Paths, agent: DaemonAgent,
                              loginItem: LoginItem?) -> Int32 {
        let requestsDir = paths.supportDir.appendingPathComponent("requests")
        let url = URL(fileURLWithPath: requestPath)

        let envelope: Desktop.Envelope
        do {
            envelope = try Desktop.readRequest(at: url, requestsDir: requestsDir)
        } catch {
            // Answer only when the reply location itself is trustworthy, so a
            // bad path can never make the app write elsewhere.
            guard let (id, replyURL) = Desktop.replyURL(forRequest: url, requestsDir: requestsDir) else {
                FileHandle.standardError.write(Data("paw-proxy: rejected request: \(error)\n".utf8))
                return 2
            }
            return write(Desktop.Reply(id: id, outcome: .failed, detail: "\(error)"), to: replyURL)
        }

        let (outcome, detail): (Desktop.Outcome, String?)
        switch envelope.request.op {
        case .register:
            (outcome, detail) = agent.turnOn()
        case .unregister:
            (outcome, detail) = agent.turnOff()
            if outcome != .failed { try? loginItem?.setEnabled(false) }
        case .status:
            (outcome, detail) = agent.status()
        }
        return write(Desktop.Reply(id: envelope.request.id, outcome: outcome, detail: detail), to: envelope.replyURL)
    }

    private static func write(_ reply: Desktop.Reply, to url: URL) -> Int32 {
        do {
            try Desktop.writeReply(reply, to: url)
            return 0
        } catch {
            FileHandle.standardError.write(Data("paw-proxy: \(error)\n".utf8))
            return 1
        }
    }
}
