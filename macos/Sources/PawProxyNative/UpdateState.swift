/// Whether the running daemon matches the installed app bundle.
///
/// Homebrew replaces the bundle while the old daemon binary keeps running,
/// so a mismatch means "restart to finish the update".
public enum UpdateState: Equatable, Sendable {
    case current
    case restartNeeded(running: String)

    public static func compare(
        bundleVersion: String, bundleBuild: String,
        daemonVersion: String, daemonBuild: String
    ) -> UpdateState {
        if isDev(bundleVersion) || isDev(daemonVersion) { return .current }
        if bundleVersion != daemonVersion {
            return .restartNeeded(running: describe(daemonVersion, daemonBuild))
        }
        // A daemon from before build reporting (or a standalone build) has no
        // build number; equal versions are good enough then.
        if daemonBuild.isEmpty || bundleBuild.isEmpty || daemonBuild == bundleBuild { return .current }
        return .restartNeeded(running: describe(daemonVersion, daemonBuild))
    }

    static func isDev(_ version: String) -> Bool {
        version.isEmpty || version == "dev" || version.hasPrefix("ci-") || version.hasSuffix("-dev")
    }

    static func describe(_ version: String, _ build: String) -> String {
        build.isEmpty ? version : "\(version) (build \(build))"
    }
}
