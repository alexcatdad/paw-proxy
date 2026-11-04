import Foundation

/// Read-only check that `sudo paw-proxy setup` has run.
public struct SetupProbe: Sendable {
    public var paths: Paths
    public init(paths: Paths) { self.paths = paths }

    public var isComplete: Bool {
        let fm = FileManager.default
        return fm.fileExists(atPath: paths.caCertificate.path) && fm.fileExists(atPath: paths.resolver().path)
    }
}
