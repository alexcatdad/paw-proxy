// swift-tools-version:6.0
import PackageDescription

let menubarKit: Target.Dependency = .product(name: "MenubarKit", package: "menubar-kit")

let package = Package(
    name: "PawProxy",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "PawProxy", targets: ["PawProxy"]),
    ],
    dependencies: [
        // First-party tray, panel and notification building blocks.
        // menubar-kit v2026.11.05.1. SwiftPM cannot resolve four-part CalVer
        // tags, so the tag's commit is pinned instead.
        .package(url: "https://github.com/alexcatdad/menubar-kit.git",
                 revision: "1e63329e0f4c42f0ed5dd4f629f7b7f0b0bac7f1"),
    ],
    targets: [
        // All testable logic lives in PawProxyNative; the app target is UI glue.
        .target(name: "PawProxyNative", dependencies: [menubarKit]),
        .executableTarget(name: "PawProxy", dependencies: ["PawProxyNative", menubarKit]),
        .testTarget(name: "PawProxyNativeTests", dependencies: ["PawProxyNative", menubarKit]),
    ]
)
