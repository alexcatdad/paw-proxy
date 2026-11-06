import Foundation
import PawProxyNative

/// Process entry. A launch with `--request <file>` comes from the CLI via
/// `open -n` (a second, fresh instance): it handles the one-shot request and
/// exits before any SwiftUI scene exists, so it never adds a second
/// menu-bar icon. Every other launch is the menu-bar app.
@main
enum Entry {
    static func main() {
        let args = CommandLine.arguments
        if let i = args.firstIndex(of: "--request") {
            guard i + 1 < args.count else {
                FileHandle.standardError.write(Data("paw-proxy: --request needs a file\n".utf8))
                exit(2)
            }
            exit(handleRequest(path: args[i + 1]))
        }
        PawProxyApp.main()
    }

    static func handleRequest(path: String) -> Int32 {
        let services = Services.live()
        return RequestHandler.handle(
            requestPath: path, paths: services.paths,
            agent: services.agent, loginItem: services.loginItem)
    }
}

/// Production wiring shared by the menu-bar app and request launches.
struct Services {
    var paths: Paths
    var isDevelopment: Bool
    var registrar: Registrar
    var launchctl: LaunchctlRunner
    var loginItem: LoginItem?
    var agent: DaemonAgent

    static func live() -> Services {
        let paths = Paths()
        let isDevelopment = Identity.isDevelopment(bundleID: Bundle.main.bundleIdentifier)
        let registrar: Registrar = isDevelopment ? DisabledRegistrar() : AgentRegistrar()
        let launchctl = SystemLaunchctl()
        let agent = DaemonAgent(paths: paths, registrar: registrar, launchctl: launchctl)
        return Services(
            paths: paths, isDevelopment: isDevelopment, registrar: registrar, launchctl: launchctl,
            loginItem: isDevelopment ? nil : MainAppLoginItem(), agent: agent)
    }
}
