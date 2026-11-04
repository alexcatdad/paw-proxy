// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "PawProxy",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "PawProxy", targets: ["PawProxy"]),
    ],
    targets: [
        // All testable logic lives in PawProxyNative; the app target is UI glue.
        .target(name: "PawProxyNative"),
        .executableTarget(name: "PawProxy", dependencies: ["PawProxyNative"]),
        .testTarget(name: "PawProxyNativeTests", dependencies: ["PawProxyNative"]),
    ]
)
