import Foundation
#if canImport(ServiceManagement)
import ServiceManagement
#endif

/// Registration state of the app-owned daemon agent.
public enum RegistrationState: Equatable, Sendable {
    case enabled
    case requiresApproval
    case notRegistered
    case notFound
    /// Development builds never register services.
    case disabled
}

/// Abstraction over SMAppService so status logic can be tested.
public protocol Registrar: Sendable {
    var state: RegistrationState { get }
    func register() throws
    func unregister() throws
    func openLoginItems()
}

public struct DisabledRegistrar: Registrar {
    public struct DevBuild: Error, CustomStringConvertible {
        public var description: String { "Dev build — registration disabled" }
    }
    public init() {}
    public var state: RegistrationState { .disabled }
    public func register() throws { throw DevBuild() }
    public func unregister() throws { throw DevBuild() }
    public func openLoginItems() {}
}

#if canImport(ServiceManagement)
/// Wraps SMAppService.agent for the bundled daemon plist.
public struct AgentRegistrar: Registrar {
    public var plistName: String
    public init(plistName: String = Identity.daemonPlistName) { self.plistName = plistName }

    private var service: SMAppService { SMAppService.agent(plistName: plistName) }

    public var state: RegistrationState {
        switch service.status {
        case .enabled: return .enabled
        case .requiresApproval: return .requiresApproval
        case .notRegistered: return .notRegistered
        case .notFound: return .notFound
        @unknown default: return .notFound
        }
    }

    public func register() throws { try service.register() }
    public func unregister() throws { try service.unregister() }
    public func openLoginItems() { SMAppService.openSystemSettingsLoginItems() }
}
#endif
