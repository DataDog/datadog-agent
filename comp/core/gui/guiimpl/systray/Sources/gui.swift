import Cocoa

class AgentGUI: NSObject, NSUserInterfaceValidations {
    let systemTrayItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    let ddMenu = NSMenu(title: "Menu")
    var versionItem: NSMenuItem!
    var openGUIItem: NSMenuItem!
    var startItem: NSMenuItem!
    var stopItem: NSMenuItem!
    var restartItem: NSMenuItem!
    var loginItem: NSMenuItem!
    var exitItem: NSMenuItem!
    let numberItems = 7
    var countUpdate: Int
    var agentStatus: Bool!
    var loginStatus: Bool!
    var updatingAgent: Bool!
    var agentRestart: Bool!
    var loginStatusEnableTitle = "Enable at login"
    var loginStatusDisableTitle = "Disable at login"

    // WiFi IPC components
    var wifiDataProvider: WiFiDataProvider?
    var wifiIPCServer: WiFiIPCServer?

    override init() {
        // make sure the first evaluation of menu item validity actually updates the items
        countUpdate = numberItems

        super.init()

        // Initialize WiFi components
        Logger.info("Initializing WiFi IPC components...", context: "AgentGUI")
        wifiDataProvider = WiFiDataProvider()
        if let provider = wifiDataProvider {
            wifiIPCServer = WiFiIPCServer(wifiDataProvider: provider)
        }

        // Create menu items
        versionItem = NSMenuItem(title: "Datadog Agent", action: nil, keyEquivalent: "")
        versionItem.isEnabled = false
        openGUIItem = NSMenuItem(title: "Open Web UI", action: #selector(openGUI), keyEquivalent: "")
        openGUIItem.target = self
        startItem = NSMenuItem(title: "Start", action: #selector(startAgent), keyEquivalent: "")
        startItem.target = self
        stopItem = NSMenuItem(title: "Stop", action: #selector(stopAgent), keyEquivalent: "")
        stopItem.target = self
        restartItem = NSMenuItem(title: "Restart", action: #selector(restartAgent), keyEquivalent: "")
        restartItem.target = self
        loginItem = NSMenuItem(title: loginStatusEnableTitle, action: #selector(loginAction), keyEquivalent: "")
        loginItem.target = self
        exitItem = NSMenuItem(title: "Exit", action: #selector(exitGUI), keyEquivalent: "")
        exitItem.target = self

        ddMenu.autoenablesItems = true
        ddMenu.addItem(versionItem)
        ddMenu.addItem(NSMenuItem.separator())
        ddMenu.addItem(openGUIItem)
        ddMenu.addItem(NSMenuItem.separator())
        ddMenu.addItem(startItem)
        ddMenu.addItem(stopItem)
        ddMenu.addItem(restartItem)
        ddMenu.addItem(loginItem)
        ddMenu.addItem(exitItem)

        // Find and load tray image
        var imagePath = "./agent.png"
        if !FileManager.default.isReadableFile(atPath: imagePath) {
            // fall back to image in applications dir
            imagePath = "/Applications/Datadog Agent.app/Contents/MacOS/agent.png"
        }
        let ddImage = NSImage(byReferencingFile: imagePath)

        // Create tray icon and set it up
        systemTrayItem.menu = ddMenu
        if ddImage!.isValid {
            ddImage!.size = NSMakeSize(15, 15)
            ddImage!.isTemplate = true
            systemTrayItem.button!.image = ddImage
        } else {
            systemTrayItem.button!.title = "DD"
        }
    }

    func validateUserInterfaceItem(_ item: NSValidatedUserInterfaceItem) -> Bool {
        // Called by Cocoa for every menu item whenever there is an update on any menu item/the menu itself.
        // Count to actually check the agent status only once for all the menu items.
        self.countUpdate += 1
        if (self.countUpdate >= self.numberItems){
            if (self.updatingAgent) {
                disableActionItems()
            } else {
                self.countUpdate = 0
                DispatchQueue.global().async {
                    self.agentStatus = AgentManager.status()
                    DispatchQueue.main.async(execute: {
                        self.updateMenuItems()
                        })
                    }
            }
        }

        if let menuItem = item as? NSMenuItem {
            return menuItem.isEnabled
        }

        return false
    }

    func run() {
        // Initialising
        agentStatus = AgentManager.status()
        loginStatus = AgentManager.getLoginStatus()
        setLoginItemState(state: AgentManager.checkCurrentInstallationMode())
        updateLoginItem()
        updatingAgent = false
        agentRestart = false
        if !agentStatus {
            // Start the Agent on App startup
            self.commandAgentService(command: "start", display: "starting")
        }

        // Start WiFi IPC server
        if let server = wifiIPCServer {
            do {
                try server.start()
                Logger.info("WiFi IPC server started successfully", context: "AgentGUI")
            } catch {
                Logger.error("Failed to start WiFi IPC server: \(error.localizedDescription)", context: "AgentGUI")
                Logger.error("WiFi metrics will be unavailable for the agent", context: "AgentGUI")
            }
        }

        NSApp.run()
    }

    func disableActionItems(){
        openGUIItem.isEnabled = false
        startItem.isEnabled = false
        stopItem.isEnabled = false
        restartItem.isEnabled = false
    }

    func updateMenuItems() {
        versionItem!.title = "Datadog Agent"
        openGUIItem.isEnabled = self.agentStatus
        startItem.isEnabled = !self.agentStatus
        stopItem.isEnabled = self.agentStatus
        restartItem.isEnabled = self.agentStatus
    }

    func updateLoginItem() {
        loginItem.title = loginStatus! ? loginStatusDisableTitle : loginStatusEnableTitle
    }

    func setLoginItemState(state: Bool) {
        self.loginItem.isEnabled = state
    }

    @objc func loginAction(_ sender: Any?) {
        self.loginStatus = AgentManager.switchLoginStatus()
        setLoginItemState(state: AgentManager.checkCurrentInstallationMode())
        updateLoginItem()
    }

    @objc func startAgent(_ sender: Any?) {
        // Verify no stale process is running before starting
        self.waitForProcessTerminationAndStart()
    }

    @objc func stopAgent(_ sender: Any?) {
        self.commandAgentService(command: "stop", display: "stopping")
    }

    @objc func restartAgent(_ sender: Any?) {
        self.agentRestart = true
        self.commandAgentService(command: "stop", display: "stopping")
    }

    @objc func openGUI(_ sender: Any?) {
        AgentManager.agentCommand(command: "launch-gui")
    }

    func commandAgentService(command: String, display: String) {
        self.updatingAgent = true
        versionItem!.title = String(format: "Datadog Agent (%@...)", display)
        self.disableActionItems()

        DispatchQueue.main.async {
            AgentManager.lifecycleCommand(command: command, callback: self.agentServiceCommandCompleted)
        }
    }

    func agentServiceCommandCompleted(agentStatus: Bool) {
        // Updating the menu items after completion
        self.updatingAgent = false
        self.agentStatus = agentStatus
        self.updateMenuItems()
        if self.agentRestart {
            self.agentRestart = false
            if !agentStatus {
                // Wait for process to fully terminate before restarting
                self.waitForProcessTerminationAndStart()
            }
        }
    }

    // Wait for agent process to fully terminate, then start
    func waitForProcessTerminationAndStart(retries: Int = 20) {
        // Check if agent process is still running
        if AgentManager.isProcessStillRunning() && retries > 0 {
            // Process still cleaning up, check again in 500ms
            Logger.debug("Agent process still running, waiting for termination... (retries left: \(retries))", context: "AgentGUI")
            DispatchQueue.main.asyncAfter(deadline: .now() + .milliseconds(500), execute: {
                self.waitForProcessTerminationAndStart(retries: retries - 1)
            })
        } else if retries > 0 {
            // Process fully terminated, safe to start
            Logger.info("Agent process terminated, starting now...", context: "AgentGUI")
            self.commandAgentService(command: "start", display: "starting")
        } else {
            // Timeout after 10 seconds (20 retries * 500ms)
            Logger.error("WARNING: Timeout waiting for agent termination, forcing start...", context: "AgentGUI")
            self.commandAgentService(command: "start", display: "starting")
        }
    }

    @objc func exitGUI(_ sender: Any?) {
        // Stop WiFi IPC server before exiting
        wifiIPCServer?.stop()
        Logger.info("WiFi IPC server stopped", context: "AgentGUI")
        NSApp.terminate(sender)
    }
}

class AgentManager {
    static let agentServiceName = "com.datadoghq.agent"
    static let userAgentPlistPath: String = "~/Library/LaunchAgents/com.datadoghq.agent.plist"
    static let serviceTimeout = 10000  // time to wait for service to start/stop, in milliseconds
    static let statusCheckFrequency = 500  // time to wait between checks on the service status, in milliseconds

    static func status() -> Bool {
        let (exitCode, stdOut, stdErr) = call(launchPath: "/bin/launchctl", arguments: ["list", agentServiceName])

        if exitCode != 0 {
            Logger.error("Command failed - stdout: \(stdOut), stderr: \(stdErr)", context: "AgentManager")
            return false
        }

        if stdOut.range(of: "\"PID\"") != nil {
            return true
        }

        return false
    }

    // Check if agent process is actually running (not just launchctl status)
    static func isProcessStillRunning() -> Bool {
        // Check for datadog-agent process using pgrep
        // Look for the actual agent binary, not the wrapper scripts
        let processInfo = bashCall(command: "pgrep -f 'datadog-agent/bin/agent/agent' | head -1")
        let hasProcess = processInfo.exitCode == 0 &&
                        !processInfo.stdOut.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty

        return hasProcess
    }

    // Run the lifecycle command (start or stop) and call the callback once the desired state is achieved
    // or a timeout is reached
    static func lifecycleCommand(command: String, callback: @escaping (Bool) -> Void) {
        let processInfo = agentServiceCall(command: command)
        if processInfo.exitCode != 0 {
            Logger.error("Command failed - stdout: \(processInfo.stdOut), stderr: \(processInfo.stdErr)", context: "AgentManager")
        }


        checkStatusAndCall(command: command, timeout: serviceTimeout, callback: callback)
    }

    static func checkCurrentInstallationMode() -> Bool {
        let process = bashCall(command: "test -f " + userAgentPlistPath)
        // True : Single User mode, False : Systemwide Install
        return process.exitCode == 0 
    }

    static func agentCommand(command: String) {
        let processInfo = agentCall(command: command)
        if processInfo.exitCode != 0 {
            Logger.error("Command failed - stdout: \(processInfo.stdOut), stderr: \(processInfo.stdErr)", context: "AgentManager")
        }
    }

    static func switchLoginStatus() -> Bool {
        let currentLoginStatus = getLoginStatus()
        var command: String
        if currentLoginStatus { // enabled -> disable
            // `unload` stops a service running for the current user session, the `-w` flag will disable it going forward
            command = "/bin/launchctl unload -w " + userAgentPlistPath
        } else { // disabled -> enable
            // `load` starts a service running for the current user session, the `-w` flag will enable it going forward
            command = "/bin/launchctl load -w " + userAgentPlistPath
        }
        let processInfo = bashCall(command: command)
        if processInfo.exitCode != 0 {
            Logger.error("Command failed - stdout: \(processInfo.stdOut), stderr: \(processInfo.stdErr)", context: "AgentManager")
            return currentLoginStatus
        }

        return !currentLoginStatus
    }

    static func getLoginStatus() -> Bool {
        let userUIDInfo = bashCall(command: "echo $UID")
        let userUID = userUIDInfo.stdOut.replacingOccurrences(of: "\n", with: "")
        let cmd = "print gui/" + userUID + "/" + agentServiceName
        let processInfo = agentCustomServiceCall(command: cmd)
        return processInfo.exitCode == 0
    }

    private static func checkStatusAndCall(command: String, timeout: Int, callback: @escaping (Bool) -> Void) {
        let agentStatus = status()
        if command == "start" && agentStatus ||
          command == "stop" && !agentStatus ||
          timeout <= 0 {
            // state change completed successfully, call callback
            DispatchQueue.main.async(execute: {
                callback(agentStatus)
            })
        } else {
            // state change not complete yet, re-check in 500 milliseconds
            DispatchQueue.main.asyncAfter(deadline: .now() + .milliseconds(statusCheckFrequency), execute: {
                checkStatusAndCall(command: command, timeout: timeout-statusCheckFrequency, callback: callback)
            })
        }
    }

    private static func agentServiceCall(command: String) -> (exitCode: Int32, stdOut: String, stdErr: String) {
        return call(launchPath: "/bin/launchctl", arguments: [command, agentServiceName])
    }

    private static func agentCustomServiceCall(command: String) -> (exitCode: Int32, stdOut: String, stdErr: String) {
        return call(launchPath: "/bin/launchctl", arguments: command.components(separatedBy: " "))
    }

    private static func agentCall(command: String) -> (exitCode: Int32, stdOut: String, stdErr: String) {
        return call(launchPath: "/usr/local/bin/datadog-agent", arguments: [command])
    }

    private static func bashCall(command: String) -> (exitCode: Int32, stdOut: String, stdErr: String) {
        return call(launchPath: "/bin/bash", arguments: ["-c", command])
    }

    private static func call(launchPath: String, arguments: [String]) -> (exitCode: Int32, stdOut: String, stdErr: String) {
        let stdOutPipe = Pipe()
        let stdErrPipe = Pipe()
        let process = Process()
        process.launchPath = launchPath
        process.arguments = arguments
        process.standardOutput = stdOutPipe
        process.standardError = stdErrPipe
        process.launch()
        process.waitUntilExit()
        let stdOut = String(data: stdOutPipe.fileHandleForReading.readDataToEndOfFile(), encoding: String.Encoding.utf8)
        let stdErr = String(data: stdErrPipe.fileHandleForReading.readDataToEndOfFile(), encoding: String.Encoding.utf8)

        return (process.terminationStatus, stdOut!, stdErr!)
    }
}
