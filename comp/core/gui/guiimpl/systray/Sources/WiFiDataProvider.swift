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

/// WiFiDataProvider handles CoreLocation permissions and WiFi data collection.
/// The permission prompt (open System Settings + dialog) is gated by request_location_permission
/// from the WLAN check config (agent configcheck wlan; agent must be running).
class WiFiDataProvider: NSObject, CLLocationManagerDelegate {
    // Note: LaunchAgent-launched apps cannot trigger location permission prompts
    // on macOS (prompts are auto-denied by the system). Permission must be granted
    // manually via System Settings -> Privacy & Security -> Location Services.
    // During the launch time of this GUI app, we will attempt to prompt for permission
    // based on the availability of the GUI environment.
    private static let userDefaultsPromptCountKey = "locationPermissionPromptAttemptCount"
    private let locationManager: CLLocationManager
    private var permissionPromptProcess: Process? = nil
    private var permissionAttemptCount: Int = 0  // Track number of permission prompt attempts (max 2), persisted per-user
    private let initializationTime: Date = Date()  // Track when WiFiDataProvider was initialized

    /// Cached value of request_location_permission from WLAN check (nil = not yet fetched).
    private static var cachedRequestLocationPermissionEnabled: Bool? = nil

    /// Tracks whether we already failed once when fetching the flag (allows one retry before caching the default).
    private static var requestLocationPermissionFetchFailedOnce: Bool = false

    /// Default when agent configcheck wlan fails (e.g. agent not running). Case 2: do not show prompt.
    private static let defaultRequestLocationPermissionWhenFetchFails = false

    /// Default when configcheck succeeds but request_location_permission is missing or unparseable in WLAN config. Case 1: show prompt.
    private static let defaultRequestLocationPermissionWhenMissing = true

    /// Returns the path to the agent binary (uses DD_INSTALL_PATH from LaunchAgent plist or default).
    private static func agentBinaryPath() -> String {
        let installPath = ProcessInfo.processInfo.environment["DD_INSTALL_PATH"] ?? "/opt/datadog-agent"
        return "\(installPath)/bin/agent/agent"
    }

    /// Runs the agent binary with arguments ["configcheck", "wlan", "-j"] and returns stdout on success (exit 0), nil on failure.
    private static func runAgentConfig() -> String? {
        let path = agentBinaryPath()
        let task = Process()
        task.launchPath = path
        task.arguments = ["configcheck", "wlan", "-j"]

        let outPipe = Pipe()
        let errPipe = Pipe()
        task.standardOutput = outPipe
        task.standardError = errPipe

        do {
            try task.run()
            task.waitUntilExit()
            guard task.terminationStatus == 0 else {
                Logger.debug("agent configcheck wlan exited with \(task.terminationStatus)", context: "WiFiDataProvider")
                return nil
            }
            let data = outPipe.fileHandleForReading.readDataToEndOfFile()
            return String(data: data, encoding: .utf8)
        } catch {
            Logger.debug("Failed to run agent configcheck wlan: \(error)", context: "WiFiDataProvider")
            return nil
        }
    }

    /// Configcheck JSON item: init_config is the WLAN check init_config (YAML string).
    private struct ConfigCheckItem: Codable {
        let init_config: String?
    }

    /// Parses request_location_permission from WLAN check init_config (from configcheck wlan JSON).
    /// Returns true if key present and value "true", false if key present and value "false", nil if missing or unparseable.
    private static func parseRequestLocationPermission(from jsonOutput: String) -> Bool? {
        guard let data = jsonOutput.data(using: .utf8) else { return nil }
        let decoder = JSONDecoder()
        guard let items = try? decoder.decode([ConfigCheckItem].self, from: data),
              let first = items.first,
              let initConfig = first.init_config, !initConfig.isEmpty else {
            return nil
        }
        let pattern = #"^\s*request_location_permission\s*:\s*(true|false)\s*($|#)"#
        guard let regex = try? NSRegularExpression(pattern: pattern, options: [.anchorsMatchLines, .caseInsensitive]) else {
            return nil
        }
        let range = NSRange(initConfig.startIndex..., in: initConfig)
        guard let match = regex.firstMatch(in: initConfig, options: [], range: range),
              let valueRange = Range(match.range(at: 1), in: initConfig) else {
            return nil
        }
        return initConfig[valueRange].lowercased() == "true"
    }

    /// Returns true if the location permission prompt is allowed (request_location_permission from WLAN check via agent configcheck wlan). Cached for process lifetime with one retry on failure.
    /// When fetch fails uses defaultRequestLocationPermissionWhenFetchFails; when flag missing uses defaultRequestLocationPermissionWhenMissing.
    private func requestLocationPermissionEnabled() -> Bool {
        if let cached = Self.cachedRequestLocationPermissionEnabled {
            Logger.debug("request_location_permission: \(cached) (cached)", context: "WiFiDataProvider")
            return cached
        }
        guard let output = Self.runAgentConfig() else {
            if Self.requestLocationPermissionFetchFailedOnce {
                Self.cachedRequestLocationPermissionEnabled = Self.defaultRequestLocationPermissionWhenFetchFails
                Logger.debug("agent configcheck wlan failed again, caching default (\(Self.defaultRequestLocationPermissionWhenFetchFails))", context: "WiFiDataProvider")
                Logger.info("request_location_permission: \(Self.defaultRequestLocationPermissionWhenFetchFails) (default, unable to get value)", context: "WiFiDataProvider")
            } else {
                Self.requestLocationPermissionFetchFailedOnce = true
                Logger.debug("agent configcheck wlan failed (will retry once on next check)", context: "WiFiDataProvider")
                Logger.debug("request_location_permission: \(Self.defaultRequestLocationPermissionWhenFetchFails) (default, fetch failed)", context: "WiFiDataProvider")
            }
            return Self.defaultRequestLocationPermissionWhenFetchFails
        }
        Self.requestLocationPermissionFetchFailedOnce = false
        let parsed = Self.parseRequestLocationPermission(from: output)
        if let value = parsed {
            Self.cachedRequestLocationPermissionEnabled = value
            Logger.info("request_location_permission: \(value) (from WLAN config)", context: "WiFiDataProvider")
            return value
        }
        let defaultWhenMissing = Self.defaultRequestLocationPermissionWhenMissing
        Self.cachedRequestLocationPermissionEnabled = defaultWhenMissing
        Logger.info("request_location_permission: \(defaultWhenMissing) (default, flag missing in WLAN config)", context: "WiFiDataProvider")
        return defaultWhenMissing
    }

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
                Logger.info("Location permission prompt disabled by agent config (request_location_permission: false) - skipping prompt", context: "WiFiDataProvider")
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
