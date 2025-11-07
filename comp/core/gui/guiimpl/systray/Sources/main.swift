import Cocoa

// Creates shared application, accessible through the NSApp variable.
let app = NSApplication.shared

// NOTE: Do NOT call setActivationPolicy(.accessory) here!
// .accessory prevents permission prompts from appearing
// With LSUIElement=false, the app will be a menu bar app but can show prompts

let agentGUI = AgentGUI()
agentGUI.run()
