import Cocoa

// Check if another instance is already running
let runningInstances = NSRunningApplication.runningApplications(
    withBundleIdentifier: "com.datadoghq.agent"
)
if runningInstances.count > 1 {
    NSLog("[GUI] Another instance is already running, exiting")
    exit(0)
}

// Creates shared application, accessible through the NSApp variable.
let app = NSApplication.shared

// Configure as background menu bar utility (hidden from Dock)
NSApp.setActivationPolicy(.accessory)

let agentGUI = AgentGUI()
agentGUI.run()
