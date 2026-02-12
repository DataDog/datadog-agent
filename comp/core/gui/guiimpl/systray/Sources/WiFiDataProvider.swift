import Foundation
import CoreLocation
import CoreWLAN

/// WiFiData represents the WiFi connection information
struct WiFiData: Codable {
    let rssi: Int
    let ssid: String
    let bssid: String
    let channel: Int
    let noise: Int
    let noiseValid: Bool
    let transmitRate: Double
    let receiveRate: Double
    let receiveRateValid: Bool
    let macAddress: String
    let phyMode: String
    let locationAuthorized: Bool
    let error: String?

    enum CodingKeys: String, CodingKey {
        case rssi
        case ssid
        case bssid
        case channel
        case noise
        case noiseValid = "noise_valid"
        case transmitRate = "transmit_rate"
        case receiveRate = "receive_rate"
        case receiveRateValid = "receive_rate_valid"
        case macAddress = "mac_address"
        case phyMode = "phy_mode"
        case locationAuthorized = "location_authorized"
        case error
    }
}

/// WiFiDataProvider handles CoreLocation permissions and WiFi data collection
class WiFiDataProvider: NSObject, CLLocationManagerDelegate {
    // Note: LaunchAgent-launched apps cannot trigger location permission prompts
    // on macOS (prompts are auto-denied by the system). Permission must be granted
    // manually via System Settings -> Privacy & Security -> Location Services.
    // During the launch time of thhis GUI app, we will attempt to prompt for permission
    // based on the availability of the GUI environment.
    private let locationManager: CLLocationManager
    private var permissionPromptProcess: Process? = nil
    private var sessionPromptAttempted: Bool = false
    private var firstWiFiRequestMade: Bool = false
    private var tccLoadingCompleted: Bool = false  // Tracks if TCC polling has finished

    override init() {
        self.locationManager = CLLocationManager()
        super.init()
        // Keep delegate to monitor permission status changes
        self.locationManager.delegate = self
        
        Logger.info("WiFiDataProvider initialized, waiting for TCC to load...", context: "WiFiDataProvider")
        
        // Poll for TCC database to load on background thread (non-blocking)
        // This provides fast permission prompts (~200ms) while avoiding the TCC timing race condition
        // The first WiFi request check serves as a safety net and second-chance mechanism
        DispatchQueue.global(qos: .utility).async { [weak self] in
            self?.waitForTCCLoad(timeout: 2.0)
        }
    }

    /// Check if GUI environment is available (can display dialogs)
    /// Returns true if running in an Aqua (GUI) session, false for Background/SSH/daemon sessions
    /// Uses launchctl to avoid automation permission prompts triggered by commands like osascript
    private func isGUIAvailable() -> Bool {
        let task = Process()
        task.launchPath = "/bin/launchctl"
        task.arguments = ["managername"]
        
        let pipe = Pipe()
        task.standardOutput = pipe
        task.standardError = Pipe() // discard errors, keep stderr silent (for cleaner logs)

        do {
            try task.run()
            task.waitUntilExit()

            guard task.terminationStatus == 0 else {
                Logger.error("launchctl managername failed (exit code: \(task.terminationStatus))", context: "WiFiDataProvider")
                return false
            }

            let data = pipe.fileHandleForReading.readDataToEndOfFile()
            let output = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""

            Logger.info("Session manager type: \(output)", context: "WiFiDataProvider")

            // Only "Aqua" indicates a GUI session where dialogs can be displayed
            // Other values: "Background" (SSH/daemon), "LoginWindow" (login screen)
            return output == "Aqua"

        } catch {
            Logger.error("Failed to check session type: \(error)", context: "WiFiDataProvider")
            return false
        }
    }

    /// Attempt to prompt for location permission via detached process
    /// Background: LaunchAgent-launched apps cannot normally trigger prompts
    private func attemptPermissionPrompt() {
        let authStatus = getAuthorizationStatus()
        
        // Skip if permission already granted
        guard authStatus != .authorizedAlways else {
            return
        }

        // Skip if permission is restricted by policy (MDM, parental controls, etc.)
        // User cannot override policy restrictions
        if authStatus == .restricted {
            Logger.info("Location permission restricted by device policy - cannot prompt", context: "WiFiDataProvider")
            Logger.info("Contact your system administrator to enable location access", context: "WiFiDataProvider")
            return
        }

        // For .notDetermined and .denied: Allow up to 2 prompt attempts
        // These are user-controllable states (unlike .restricted)
        // Only show once per session (prevents repeated prompts after dismissal)
        guard !sessionPromptAttempted else {
            Logger.debug("Permission prompt already attempted this session, skipping", context: "WiFiDataProvider")
            return
        }

        // Don't spawn duplicate if dialog process is currently running
        if let process = permissionPromptProcess, process.isRunning {
            Logger.debug("Permission dialog process still running, skipping duplicate", context: "WiFiDataProvider")
            return
        }

        Logger.info("Attempting to prompt for location permission via detached process...", context: "WiFiDataProvider")

        // Spawn detached osascript process to show dialog and open System Settings
        let script = """
        do shell script "open 'x-apple.systempreferences:com.apple.preference.security?Privacy_LocationServices'" & ¬
        " > /dev/null 2>&1 &"
        
        display dialog "Datadog Agent requires Location permission to collect WiFi network information (SSID/BSSID).
        
        Please:
        1. In the System Settings window that just opened, scroll to 'Datadog Agent'
        2. Enable the toggle switch next to 'Datadog Agent'
        3. Click OK below when done
        
        Note: This prompt may auto-close if the system denies permission requests from background apps." ¬
        buttons {"OK"} ¬
        default button "OK" ¬
        with title "Enable Location Permission" ¬
        with icon caution
        """
        
        // Run osascript in background (detached from current process)
        let task = Process()
        task.launchPath = "/usr/bin/osascript"
        task.arguments = ["-e", script]
        task.standardOutput = nil
        task.standardError = nil
        
        do {
            try task.run()
            // Don't wait for completion - let it run detached
            permissionPromptProcess = task
            sessionPromptAttempted = true  // Only set on successful spawn
            Logger.info("Permission prompt process spawned (PID: \(task.processIdentifier))", context: "WiFiDataProvider")
        } catch {
            Logger.error("Failed to spawn permission prompt: \(error)", context: "WiFiDataProvider")
            permissionPromptProcess = nil
            // sessionPromptAttempted stays false - can retry later
        }
    }

    /// Wait for TCC (Transparency, Consent, and Control) database to load
    /// TCC loads asynchronously when CLLocationManager is created, typically taking 50-500ms
    /// This method polls the authorization status until it becomes definitive (not .notDetermined)
    /// or until the timeout is reached
    private func waitForTCCLoad(timeout: TimeInterval = 2.0) {
        let startTime = Date()
        let pollInterval: TimeInterval = 0.05 // 50ms - balance between responsiveness and CPU usage

        // Early exit optimization: check immediately first
        // On warm start, TCC might already be cached and ready
        let initialStatus = getAuthorizationStatus()
        if initialStatus != .notDetermined {
            let elapsed = Int(Date().timeIntervalSince(startTime) * 1000)
            Logger.info("TCC already loaded (immediate check after \(elapsed)ms), status: \(authorizationStatusString())", context: "WiFiDataProvider")

            DispatchQueue.main.async { [weak self] in
                guard let self = self else { return }
                self.handleTCCLoadedStatus(initialStatus)
            }
            return
        }

        // TCC not ready yet, start polling
        Logger.info("TCC not ready (initial status: notDetermined), polling for up to \(Int(timeout * 1000))ms...", context: "WiFiDataProvider")

        while Date().timeIntervalSince(startTime) < timeout {
            let status = getAuthorizationStatus()

            // Stop polling if we have a definitive answer
            if status != .notDetermined {
                let elapsed = Int(Date().timeIntervalSince(startTime) * 1000)
                Logger.info("TCC loaded after \(elapsed)ms, status: \(authorizationStatusString())", context: "WiFiDataProvider")

                // Handle the loaded status on main thread
                DispatchQueue.main.async { [weak self] in
                    guard let self = self else { return }
                    self.handleTCCLoadedStatus(status)
                }
                return
            }

            Thread.sleep(forTimeInterval: pollInterval)
        }

        // Timeout - status still notDetermined after 2 seconds
        let elapsed = Int(timeout * 1000)
        Logger.info("TCC poll timeout after \(elapsed)ms, status: notDetermined", context: "WiFiDataProvider")
        Logger.info("Permission likely not set - will attempt prompt", context: "WiFiDataProvider")

        // Permission truly not set - handle on main thread
        DispatchQueue.main.async { [weak self] in
            guard let self = self else { return }
            self.handleTCCLoadedStatus(.notDetermined)
        }
    }

    /// Handle TCC authorization status after it has been loaded
    /// This is called from waitForTCCLoad() on the main thread
    private func handleTCCLoadedStatus(_ status: CLAuthorizationStatus) {
        // Mark TCC loading as complete - this allows Check #2 to proceed with real status
        self.tccLoadingCompleted = true

        switch status {
        case .notDetermined:
            // After 2 seconds, still notDetermined = permission truly not set
            Logger.info("Location permission not granted - SSID/BSSID will be unavailable", context: "WiFiDataProvider")
            Logger.info("To enable: System Settings → Privacy & Security → Location Services → Datadog Agent", context: "WiFiDataProvider")

            if isGUIAvailable() {
                Logger.info("GUI environment detected, attempting permission prompt...", context: "WiFiDataProvider")
                attemptPermissionPrompt()
            } else {
                Logger.info("Headless environment detected, skipping permission prompt (will retry at first WiFi request)", context: "WiFiDataProvider")
            }

        case .authorizedAlways:
            Logger.info("Location permission already granted - WiFi SSID/BSSID will be available", context: "WiFiDataProvider")

        case .denied:
            // User explicitly denied - but they can change their mind
            // Give them an opportunity to reconsider (Check #1)
            Logger.info("Location permission previously denied - SSID/BSSID will be unavailable", context: "WiFiDataProvider")
            Logger.info("To enable: System Settings → Privacy & Security → Location Services → Datadog Agent", context: "WiFiDataProvider")

            if isGUIAvailable() {
                Logger.info("Attempting to prompt (user may have changed mind)...", context: "WiFiDataProvider")
                attemptPermissionPrompt()
            } else {
                Logger.info("Headless environment detected, will retry at first WiFi request", context: "WiFiDataProvider")
            }

        case .restricted:
            // Policy restriction - cannot override, don't prompt
            Logger.info("Location permission restricted by device policy - SSID/BSSID will be unavailable", context: "WiFiDataProvider")
            Logger.info("Contact your system administrator to enable location access", context: "WiFiDataProvider")

        @unknown default:
            Logger.error("Unknown authorization status: \(status.rawValue)", context: "WiFiDataProvider")
        }
    }

    /// Get current WiFi information for the system
    func getWiFiInfo() -> WiFiData {
        // Check current authorization status (read-only, no prompt attempt)
        let authStatus = getAuthorizationStatus()
        let isAuthorized = (authStatus == .authorizedAlways)

        // Check #2 (Safety Net): On first WiFi request without permission, try prompting
        // NOTE: Only check AFTER TCC loading completes (tccLoadingCompleted flag)
        // This prevents acting on stale "notDetermined" status during the first ~2 seconds
        // The first WiFi requests that arrive before TCC loads will still work (returning RSSI, noise, etc.)
        // Only SSID/BSSID will be empty without location permission - this is acceptable
        // This serves two purposes:
        // 1. Safety net: Catches edge cases where TCC polling at startup failed/timed out
        // 2. Second chance: Allows recovery if user accidentally denied permission at startup
        if !isAuthorized && !firstWiFiRequestMade && tccLoadingCompleted {
            if isGUIAvailable() {
                Logger.info("First WiFi data request without permission (after TCC load), attempting prompt...", context: "WiFiDataProvider")
                attemptPermissionPrompt()
            } else {
                Logger.info("First WiFi data request without permission, but headless environment - skipping prompt", context: "WiFiDataProvider")
            }
            firstWiFiRequestMade = true
        }

        // Get WiFi interface (this works regardless of location permission)
        let client = CWWiFiClient.shared()

        guard let interface = client.interface() else {
            Logger.error("No WiFi interface available", context: "WiFiDataProvider")
            return createErrorData(message: "No WiFi interface", authorized: isAuthorized)
        }

        // Get PHY mode string
        let phyModeStr = phyModeToString(interface.activePHYMode())

        // Check if interface is active
        if phyModeStr == "None" {
            Logger.info("WiFi interface is not active (PHY mode: None)", context: "WiFiDataProvider")
            return WiFiData(
                rssi: 0,
                ssid: "",
                bssid: "",
                channel: 0,
                noise: 0,
                noiseValid: false,
                transmitRate: 0.0,
                receiveRate: 0.0,
                receiveRateValid: false,
                macAddress: interface.hardwareAddress() ?? "",
                phyMode: phyModeStr,
                locationAuthorized: isAuthorized,
                error: "WiFi interface not active"
            )
        }

        // Collect WiFi data (SSID/BSSID require location permission on macOS 11+)
        let ssid = interface.ssid() ?? ""
        let bssid = interface.bssid() ?? ""
        let macAddress = interface.hardwareAddress() ?? ""

        // Log if SSID/BSSID are empty (might indicate permission issue)
        if !isAuthorized && (ssid.isEmpty || bssid.isEmpty) {
            Logger.info("WARN: SSID/BSSID empty - location permission not granted", context: "WiFiDataProvider")
        }

        let wifiData = WiFiData(
            rssi: interface.rssiValue(),
            ssid: ssid,
            bssid: bssid,
            channel: interface.wlanChannel()?.channelNumber ?? 0,
            noise: interface.noiseMeasurement(),
            noiseValid: true,
            transmitRate: interface.transmitRate(),
            receiveRate: 0.0, // Not directly available in CoreWLAN API
            receiveRateValid: false,
            macAddress: macAddress,
            phyMode: phyModeStr,
            locationAuthorized: isAuthorized,
            error: nil
        )

        Logger.debug("Collected WiFi data: SSID=\(ssid.isEmpty ? "<empty>" : ssid), RSSI=\(wifiData.rssi), authorized=\(isAuthorized)", context: "WiFiDataProvider")
        return wifiData
    }

    // CLLocationManagerDelegate
    // locationManagerDidChangeAuthorization: Monitors location authorization status changes.
    // Note: This is triggered when the user manually changes permission in System Settings,
    // not from programmatic permission requests (which don't work for LaunchAgent apps).
    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = getAuthorizationStatus()
        Logger.info("Location authorization changed to: \(authorizationStatusString())", context: "WiFiDataProvider")
        
        // Reset flags when authorization status changes
        // This allows showing the prompt again if permission is revoked or status changes
        sessionPromptAttempted = false
        permissionPromptProcess = nil
        firstWiFiRequestMade = false

        switch status {
        case .authorizedAlways:
            Logger.info("Location permission GRANTED - WiFi SSID/BSSID will be available", context: "WiFiDataProvider")
        case .denied:
            Logger.info("Location permission DENIED - WiFi SSID/BSSID will be unavailable", context: "WiFiDataProvider")
        case .restricted:
            Logger.info("Location permission RESTRICTED", context: "WiFiDataProvider")
        case .notDetermined:
            Logger.info("Location permission NOT DETERMINED", context: "WiFiDataProvider")
        @unknown default:
            Logger.error("Unknown authorization status: \(status.rawValue)", context: "WiFiDataProvider")
        }
    }

    // Authorization Status Helper
    private func getAuthorizationStatus() -> CLAuthorizationStatus {
        if #available(macOS 11.0, *) {
            return locationManager.authorizationStatus
        } else {
            return CLLocationManager.authorizationStatus()
        }
    }

    // Helper Methods
    private func createErrorData(message: String, authorized: Bool) -> WiFiData {
        return WiFiData(
            rssi: 0,
            ssid: "",
            bssid: "",
            channel: 0,
            noise: 0,
            noiseValid: false,
            transmitRate: 0.0,
            receiveRate: 0.0,
            receiveRateValid: false,
            macAddress: "",
            phyMode: "None",
            locationAuthorized: authorized,
            error: message
        )
    }

    private func phyModeToString(_ mode: CWPHYMode) -> String {
        switch mode {
        case .mode11a:
            return "802.11a"
        case .mode11b:
            return "802.11b"
        case .mode11g:
            return "802.11g"
        case .mode11n:
            return "802.11n"
        case .mode11ac:
            return "802.11ac"
        case .mode11ax:
            return "802.11ax"
        case .modeNone:
            return "None"
        @unknown default:
            return "Unknown"
        }
    }

    private func authorizationStatusString() -> String {
        let status = getAuthorizationStatus()
        switch status {
        case .notDetermined:
            return "notDetermined"
        case .restricted:
            return "restricted"
        case .denied:
            return "denied"
        case .authorizedAlways:
            return "authorizedAlways"
        @unknown default:
            return "unknown(\(status.rawValue))"
        }
    }
}
