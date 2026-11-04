import Foundation

/// File locations shared with the Go daemon (internal/paths/paths_darwin.go).
public struct Paths: Sendable, Equatable {
    public var home: URL
    public var resolverDir: URL

    public init(home: URL = FileManager.default.homeDirectoryForCurrentUser,
                resolverDir: URL = URL(fileURLWithPath: "/etc/resolver")) {
        self.home = home
        self.resolverDir = resolverDir
    }

    public var supportDir: URL { home.appendingPathComponent("Library/Application Support/paw-proxy") }
    public var socket: URL { supportDir.appendingPathComponent("paw-proxy.sock") }
    public var caCertificate: URL { supportDir.appendingPathComponent("ca.crt") }
    public var log: URL { home.appendingPathComponent("Library/Logs/paw-proxy.log") }
    public func resolver(tld: String = "test") -> URL { resolverDir.appendingPathComponent(tld) }
}

/// Identifiers shared with the Go side (internal/appbundle).
public enum Identity {
    public static let bundleID = "com.alexcatdad.paw-proxy"
    public static let daemonLabel = "com.alexcatdad.paw-proxy.daemon"
    public static let legacyLabel = "dev.paw-proxy"
    public static let daemonPlistName = "com.alexcatdad.paw-proxy.daemon.plist"

    /// Development builds use a ".dev" bundle ID and never register services.
    public static func isDevelopment(bundleID: String?) -> Bool {
        bundleID?.hasSuffix(".dev") ?? false
    }
}
