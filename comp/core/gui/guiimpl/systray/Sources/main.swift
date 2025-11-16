import Cocoa

// Check if another instance is already running (exclude self)
let myPID = ProcessInfo.processInfo.processIdentifier
let runningInstances = NSRunningApplication.runningApplications(
    withBundleIdentifier: "com.datadoghq.agent"
).filter { $0.processIdentifier != myPID }

if !runningInstances.isEmpty {
    if let otherPID = runningInstances.first?.processIdentifier {
        NSLog("[GUI] Another instance is already running (PID: \(otherPID)), exiting")
    }
    exit(0)
}
// Creates shared application, accessible through the NSApp variable.
let app = NSApplication.shared

// Configure as background menu bar utility (hidden from Dock)
NSApp.setActivationPolicy(.accessory)

let agentGUI = AgentGUI()
agentGUI.run()
