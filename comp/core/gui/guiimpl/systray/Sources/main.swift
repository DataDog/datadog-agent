import Cocoa

// Parse command-line arguments to determine logging mode
let args = CommandLine.arguments
let useFileLogging = args.contains("--use-file-logging")

// Configure logging mode globally before any other operations
Logger.configure(useFileLogging: useFileLogging)

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

// Configure as background menu bar utility (hidden from Dock)
NSApp.setActivationPolicy(.accessory)

let agentGUI = AgentGUI()
agentGUI.run()
