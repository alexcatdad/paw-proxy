// swift-tools-version:6.0
import PackageDescription

let menubarKit: Target.Dependency = .product(name: "MenubarKit", package: "menubar-kit")

let package = Package(
    name: "PawProxy",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "PawProxy", targets: ["PawProxy"])
    ],
    dependencies: [
        // First-party tray, panel and notification building blocks.
        // menubar-kit v2026.11.07.1. SwiftPM cannot resolve four-part CalVer
        // tags, so the tag's commit is pinned instead.
        .package(
            url: "https://github.com/alexcatdad/menubar-kit.git",
            revision: "4027df8aaafd84a9c3dc46571477c76b5d74921d")
    ],
    targets: [
        // All testable logic lives in PawProxyNative; the app target is UI glue.
        .target(name: "PawProxyNative", dependencies: [menubarKit]),
        .executableTarget(name: "PawProxy", dependencies: ["PawProxyNative", menubarKit]),
        .testTarget(name: "PawProxyNativeTests", dependencies: ["PawProxyNative", menubarKit]),
        // Layout and render checks of the app's SwiftUI views.
        .testTarget(name: "PawProxyTests", dependencies: ["PawProxy", "PawProxyNative", menubarKit]),
    ]
)
