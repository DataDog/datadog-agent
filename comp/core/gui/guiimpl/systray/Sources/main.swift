import Cocoa

// Parse command-line arguments
let args = CommandLine.arguments
let useFileLogging = args.contains("--use-file-logging")
let headlessMode = args.contains("--headless")

// Configure logging mode globally before any other operations
Logger.configure(useFileLogging: useFileLogging)

if headlessMode {
    Logger.info("Starting in headless mode (no menu bar icon)", context: "GUI")
} else {
    Logger.info("Starting in GUI mode (menu bar icon visible)", context: "GUI")
}

// Check if another instance is already running (exclude self)
let myPID = ProcessInfo.processInfo.processIdentifier
let runningInstances = NSRunningApplication.runningApplications(
    withBundleIdentifier: "com.datadoghq.agent"
).filter { $0.processIdentifier != myPID }

if !runningInstances.isEmpty {
    if let otherPID = runningInstances.first?.processIdentifier {
        Logger.info("Another instance is already running (PID: \(otherPID)), exiting", context: "GUI")
    }
    exit(0)
}
// Creates shared application, accessible through the NSApp variable.
let app = NSApplication.shared

// Configure as background utility (hidden from Dock)
NSApp.setActivationPolicy(.accessory)

if headlessMode {
    // Headless mode: Run only WiFi IPC server, no GUI
    Logger.info("Initializing desktop ready tracker...", context: "GUI")
    let desktopReadyTracker = DesktopReadyTracker()

    Logger.info("Initializing WiFi IPC components...", context: "GUI")
    let wifiDataProvider = WiFiDataProvider()
    let wifiIPCServer = WiFiIPCServer(wifiDataProvider: wifiDataProvider, desktopReadyTracker: desktopReadyTracker)

    do {
        try wifiIPCServer.start()
        Logger.info("WiFi IPC server started successfully", context: "GUI")
    } catch {
        Logger.error("Failed to start WiFi IPC server: \(error.localizedDescription)", context: "GUI")
        exit(1)
    }

    // Keep the app running
    NSApp.run()
} else {
    // GUI mode: Run full menu bar app with WiFi IPC
    let agentGUI = AgentGUI()
    agentGUI.run()
}
