import Cocoa

// Creates shared application, accessible through the NSApp variable.
let app = NSApplication.shared

// Hide from Dock while still allowing permission prompts
// This must be called early, before any UI initialization
NSApp.setActivationPolicy(.accessory)

let agentGUI = AgentGUI()
agentGUI.run()
