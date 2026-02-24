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
    private static let userDefaultsPromptCountKey = "locationPermissionPromptAttemptCount"
    private let locationManager: CLLocationManager
    private var permissionPromptProcess: Process? = nil
    private var permissionAttemptCount: Int = 0  // Track number of permission prompt attempts (max 2), persisted per-user
    private let initializationTime: Date = Date()  // Track when WiFiDataProvider was initialized

    /// Cached value of request_location_permission from datadog.yaml (nil = not yet read).
    private static var cachedRequestLocationPermissionEnabled: Bool? = nil

    override init() {
        self.locationManager = CLLocationManager()
        super.init()
        // Keep delegate to monitor permission status changes
        self.locationManager.delegate = self

        // Load persisted attempt count (per-user, per-installation); clamp to 0...2 (negative/corrupt → 0)
        permissionAttemptCount = min(max(UserDefaults.standard.integer(forKey: Self.userDefaultsPromptCountKey), 0), 2)

        Logger.info("WiFiDataProvider initialized", context: "WiFiDataProvider")
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

    /// Resolves the path to datadog.yaml (install path + "/etc/datadog.yaml").
    /// Tries: system-wide plist, per-user plist, process env DD_INSTALL_PATH, then hardcoded fallback.
    private static func resolveDatadogConfigPath() -> String {
        let systemPlist = "/Library/LaunchAgents/com.datadoghq.gui.plist"
        let perUserPlist = (FileManager.default.homeDirectoryForCurrentUser.path as NSString).appendingPathComponent("Library/LaunchAgents/com.datadoghq.gui.plist")
        let plistPaths = [systemPlist, perUserPlist]

        for plistPath in plistPaths {
            guard FileManager.default.isReadableFile(atPath: plistPath) else { continue }
            let url = URL(fileURLWithPath: plistPath)
            guard let plist = NSDictionary(contentsOf: url) as? [String: Any],
                  let env = plist["EnvironmentVariables"] as? [String: Any],
                  let installPath = env["DD_INSTALL_PATH"] as? String,
                  !installPath.isEmpty else { continue }
            let configPath = (installPath as NSString).appendingPathComponent("etc/datadog.yaml")
            return configPath
        }

        if let envPath = ProcessInfo.processInfo.environment["DD_INSTALL_PATH"], !envPath.isEmpty {
            return (envPath as NSString).appendingPathComponent("etc/datadog.yaml")
        }
        return "/opt/datadog-agent/etc/datadog.yaml"
    }

    /// Returns true if the location permission prompt is allowed (request_location_permission in datadog.yaml).
    /// Returns true when file/key is missing or unreadable (default allow). Cached for process lifetime.
    private func requestLocationPermissionEnabled() -> Bool {
        if let cached = Self.cachedRequestLocationPermissionEnabled {
            return cached
        }
        let path = Self.resolveDatadogConfigPath()
        guard FileManager.default.isReadableFile(atPath: path),
              let content = try? String(contentsOfFile: path, encoding: .utf8) else {
            Self.cachedRequestLocationPermissionEnabled = true
            return true
        }
        // Match request_location_permission: true or false (optional comment # before key)
        let pattern = #"^#?\s*request_location_permission\s*:\s*(true|false)\s*($|#)"#
        guard let regex = try? NSRegularExpression(pattern: pattern, options: [.anchorsMatchLines, .caseInsensitive]) else {
            Self.cachedRequestLocationPermissionEnabled = true
            return true
        }
        let range = NSRange(content.startIndex..., in: content)
        guard let match = regex.firstMatch(in: content, options: [], range: range),
              let valueRange = Range(match.range(at: 1), in: content) else {
            Self.cachedRequestLocationPermissionEnabled = true
            return true
        }
        let value = String(content[valueRange]).lowercased()
        let enabled = (value != "false")
        Self.cachedRequestLocationPermissionEnabled = enabled
        return enabled
    }

    /// Attempt to prompt for location permission via detached process
    /// Background: LaunchAgent-launched apps cannot normally trigger prompts
    /// Caller must pass current auth status (call only when not authorized).
    private func attemptPermissionPrompt(authStatus: CLAuthorizationStatus) {
        // Skip if permission is restricted by policy (MDM, parental controls, etc.)
        // User cannot override policy restrictions
        if authStatus == .restricted {
            Logger.info("Location permission restricted by device policy - cannot prompt", context: "WiFiDataProvider")
            Logger.info("Contact your system administrator to enable location access", context: "WiFiDataProvider")
            return
        }

        // For .notDetermined and .denied: Allow up to 2 prompt attempts
        // These are user-controllable states (unlike .restricted)
        // This gives users a second chance if they make a wrong choice
        guard permissionAttemptCount < 2 else {
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
            permissionAttemptCount += 1  // Increment on successful spawn
            // Persist per-user so 2-attempt limit survives app restarts (never write 0)
            UserDefaults.standard.set(permissionAttemptCount, forKey: Self.userDefaultsPromptCountKey)
            Logger.info("Permission prompt process spawned (PID: \(task.processIdentifier), attempt \(permissionAttemptCount)/2)", context: "WiFiDataProvider")
        } catch {
            Logger.error("Failed to spawn permission prompt: \(error)", context: "WiFiDataProvider")
            permissionPromptProcess = nil
            // permissionAttemptCount not incremented - can retry later
        }
    }

    /// Get current WiFi information for the system
    func getWiFiInfo() -> WiFiData {
        // Check current authorization status (read-only, no prompt attempt)
        let authStatus = getAuthorizationStatus()
        let isAuthorized = (authStatus == .authorizedAlways)

        // Permission detection during WLAN data collection
        // Check permission at least 10 seconds after initialization to avoid TCC loading race condition
        // This ensures TCC database has time to load before we check permission status
        // Allow up to 2 attempts to give users a second chance if they make a wrong choice
        let timeSinceInit = Date().timeIntervalSince(initializationTime)
        if !isAuthorized && permissionAttemptCount < 2 && timeSinceInit >= 10.0 {
            if requestLocationPermissionEnabled() && isGUIAvailable() {
                Logger.info("WiFi data request without permission (after 10s delay), attempting prompt (attempt \(permissionAttemptCount + 1)/2)...", context: "WiFiDataProvider")
                attemptPermissionPrompt(authStatus: authStatus)
            } else if !requestLocationPermissionEnabled() {
                Logger.info("Location permission prompt disabled by request_location_permission in config - skipping prompt", context: "WiFiDataProvider")
            } else {
                Logger.info("WiFi data request without permission in headless environment - skipping prompt", context: "WiFiDataProvider")
            }
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

        // Clean up prompt process when authorization status changes
        // Do not reset permissionAttemptCount so we respect user's manual choice:
        // if they denied twice or later manually disable in Settings, we do not prompt again
        permissionPromptProcess = nil

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
