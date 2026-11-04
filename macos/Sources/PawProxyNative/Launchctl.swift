import Darwin
import Foundation

/// launchctl operations on the current user's GUI domain.
public protocol LaunchctlRunner: Sendable {
    /// The PID of a loaded job, or 0 when it is loaded but not running;
    /// nil when the label is not loaded.
    func pid(_ label: String) -> Int32?
    func bootout(_ label: String) throws
    func bootstrap(plist: URL) throws
    func kickstart(_ label: String) throws
}

public extension LaunchctlRunner {
    func isLoaded(_ label: String) -> Bool { pid(label) != nil }
}

public struct LaunchctlError: Error, CustomStringConvertible, Equatable {
    public var description: String
    public init(_ d: String) { description = d }
}

/// Runs /bin/launchctl against `gui/<uid>`.
public struct SystemLaunchctl: LaunchctlRunner {
    public var uid: uid_t
    public init(uid: uid_t = getuid()) { self.uid = uid }

    private var domain: String { "gui/\(uid)" }

    public func pid(_ label: String) -> Int32? {
        let r = Self.run(["print", "\(domain)/\(label)"])
        guard r.status == 0 else { return nil }
        return Self.parsePID(r.output)
    }

    public func bootout(_ label: String) throws {
        let r = Self.run(["bootout", "\(domain)/\(label)"])
        // 3/113: "No such process" / "Could not find service" — already gone.
        if r.status != 0, r.status != 3, r.status != 113, isLoaded(label) {
            throw LaunchctlError("launchctl bootout \(label) failed: \(r.output.trimmed)")
        }
    }

    public func bootstrap(plist: URL) throws {
        let r = Self.run(["bootstrap", domain, plist.path])
        if r.status != 0 { throw LaunchctlError("launchctl bootstrap failed: \(r.output.trimmed)") }
    }

    public func kickstart(_ label: String) throws {
        let r = Self.run(["kickstart", "-k", "\(domain)/\(label)"])
        if r.status != 0 { throw LaunchctlError("launchctl kickstart failed: \(r.output.trimmed)") }
    }

    /// Extracts `pid = N` from `launchctl print` output; 0 when absent.
    static func parsePID(_ output: String) -> Int32 {
        for line in output.split(separator: "\n") {
            let t = line.trimmingCharacters(in: .whitespaces)
            if t.hasPrefix("pid = "), let n = Int32(t.dropFirst(6)) { return n }
        }
        return 0
    }

    static func run(_ args: [String]) -> (status: Int32, output: String) {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        p.arguments = args
        let pipe = Pipe()
        p.standardOutput = pipe
        p.standardError = pipe
        do { try p.run() } catch { return (-1, "\(error)") }
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        return (p.terminationStatus, String(decoding: data, as: UTF8.self))
    }
}

extension String {
    var trimmed: String { trimmingCharacters(in: .whitespacesAndNewlines) }
}
