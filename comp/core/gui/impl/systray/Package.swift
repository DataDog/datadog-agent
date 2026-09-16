// swift-tools-version:5.5

import PackageDescription

let package = Package(
    name: "dd-agent-gui",
    platforms: [.macOS(.v12)], // Keep in sync with https://docs.datadoghq.com/agent/supported_platforms/?tab=macos
    targets: [
        .target(name: "dd-agent-gui", path: "./Sources")
    ]
)
