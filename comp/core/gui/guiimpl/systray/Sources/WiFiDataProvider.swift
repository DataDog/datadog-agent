import Foundation
import AppKit
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
/// The permission prompt (custom dialog + requestWhenInUseAuthorization) runs when status is .notDetermined,
/// or when .denied if DD_GUI_SHOW_LOCATION_DIALOG_WHEN_DENIED is set. Gated by request_location_permission
/// from the WLAN check config (agent configcheck wlan; agent must be running). Single attempt per user.
class WiFiDataProvider: NSObject, CLLocationManagerDelegate {
    // Note: LaunchAgent-launched apps cannot trigger location permission prompts
    // on macOS (prompts are auto-denied by the system). Permission must be granted
    // manually via System Settings -> Privacy & Security -> Location Services.
    // During the launch time of this GUI app, we will attempt to prompt for permission
    // based on the availability of the GUI environment.
    private static let userDefaultsPromptCountKey = "locationPermissionPromptAttemptCount"
    /// Serializes "should we show the prompt" so only one thread can claim the single attempt (avoids two custom dialogs).
    private static let permissionPromptLock = NSLock()
    /// Shared across all instances so only one custom dialog is ever shown (e.g. GUI and IPC may use different provider instances).
    private static var permissionAttemptCount: Int = {
        min(max(UserDefaults.standard.integer(forKey: "locationPermissionPromptAttemptCount"), 0), 1)
    }()
    private let locationManager: CLLocationManager
    private let initializationTime: Date = Date()  // Track when WiFiDataProvider was initialized

    /// Cached value of request_location_permission from WLAN check (nil = not yet fetched).
    private static var cachedRequestLocationPermissionEnabled: Bool? = nil

    /// Tracks whether we already failed once when fetching the flag (allows one retry before caching the default).
    private static var requestLocationPermissionFetchFailedOnce: Bool = false

    /// True after we have logged "Already attempted once, skipping" once (avoids repeated debug logs on every getWiFiInfo).
    private static var hasLoggedAlreadyAttemptedOnce: Bool = false

    /// Default when agent configcheck wlan fails (e.g. agent not running). Case 2: do not show prompt.
    private static let defaultRequestLocationPermissionWhenFetchFails = false

    /// Default when configcheck succeeds but request_location_permission is missing or unparseable in WLAN config. Case 1: show prompt.
    private static let defaultRequestLocationPermissionWhenMissing = true

    /// When true, the custom dialog is also shown when status is .denied (e.g. for testing). Set DD_GUI_SHOW_LOCATION_DIALOG_WHEN_DENIED=true (or 1).
    /// When user taps Allow we still call the native API; it is a no-op when .denied but keeps code simple.
    private static func showLocationDialogWhenDeniedEnabled() -> Bool {
        let v = ProcessInfo.processInfo.environment["DD_GUI_SHOW_LOCATION_DIALOG_WHEN_DENIED"]?.lowercased()
        return v == "true" || v == "1"
    }

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

    /// Requests location permission. Call only after the single attempt has been claimed under permissionPromptLock.
    /// For .notDetermined: only calls the native API (one system dialog). For .denied with flag: shows custom dialog then native API.
    private func attemptPermissionPrompt(authStatus: CLAuthorizationStatus) {
        let showCustomDialog = authStatus == .denied && Self.showLocationDialogWhenDeniedEnabled()
        guard authStatus == .notDetermined || showCustomDialog else {
            return
        }
        Logger.info("Attempting location permission prompt (single attempt), status \(authorizationStatusString())", context: "WiFiDataProvider")
        DispatchQueue.main.async { [weak self] in
            guard let self = self else { return }
            if authStatus == .notDetermined {
                if #available(macOS 10.15, *) {
                    self.locationManager.requestWhenInUseAuthorization()
                    Logger.debug("requestWhenInUseAuthorization() called (system dialog only)", context: "WiFiDataProvider")
                }
            } else {
                self.showLocationPermissionAlertAndRequestIfAllowed()
            }
        }
    }

    /// Shows an informational dialog telling the user to enable Location in System Settings.
    /// Must be called on the main thread. Single attempt per installation.
    private func showLocationPermissionAlertAndRequestIfAllowed() {
        let alert = NSAlert()
        alert.messageText = "Location Permission Required"
        alert.informativeText = "Datadog Agent needs Location permission to access WiFi network information.\n\nPlease enable it in:\nSystem Settings → Privacy & Security → Location Services → Datadog Agent.app"
        alert.alertStyle = .informational
        if let icon = Self.datadogIconImage() {
            alert.icon = icon
        }
        alert.addButton(withTitle: "OK")

        alert.runModal()

        Logger.info("Location permission info dialog dismissed", context: "WiFiDataProvider")
    }

    /// Returns the Datadog app icon for use in dialogs (same icon as DMG installation / Finder).
    /// Uses only Agent.icns so the dialog never shows the tray icon (agent.png) or other wrong assets.
    /// Picks the largest available representation for a crisp dialog icon.
    private static func datadogIconImage() -> NSImage? {
        var imagePath: String?
        // 1) DMG/app icon: Agent.icns in Contents/Resources (official build and build-app-bundle.sh)
        let bundleResources = (Bundle.main.bundlePath as NSString).appendingPathComponent("Contents/Resources/Agent.icns")
        if FileManager.default.isReadableFile(atPath: bundleResources) {
            imagePath = bundleResources
        }
        // 2) Same via Bundle API
        if imagePath == nil, let path = Bundle.main.path(forResource: "Agent", ofType: "icns") {
            imagePath = path
        }
        // 3) Installed app under /Applications (DMG icon)
        if imagePath == nil, FileManager.default.isReadableFile(atPath: "/Applications/Datadog Agent.app/Contents/Resources/Agent.icns") {
            imagePath = "/Applications/Datadog Agent.app/Contents/Resources/Agent.icns"
        }
        guard let path = imagePath else { return nil }
        guard let image = NSImage(contentsOfFile: path), image.isValid else { return nil }
        // Pick the largest representation for a crisp dialog icon (NSAlert displays at ~64pt).
        if let best = image.representations.max(by: { $0.pixelsWide < $1.pixelsWide }) {
            let size = NSSize(width: best.pixelsWide, height: best.pixelsHigh)
            let out = NSImage(size: size)
            out.addRepresentation(best)
            out.isTemplate = false
            return out
        }
        image.isTemplate = false
        return image
    }

    /// Get current WiFi information for the system
    func getWiFiInfo() -> WiFiData {
        // Check current authorization status (read-only, no prompt attempt)
        let authStatus = getAuthorizationStatus()
        let isAuthorized = (authStatus == .authorizedAlways)

        // Permission detection during WLAN data collection
        // Check permission at least 10 seconds after initialization to avoid TCC loading race condition.
        // Prompt when .notDetermined, or when .denied and DD_GUI_SHOW_LOCATION_DIALOG_WHEN_DENIED is set. Single attempt.
        let timeSinceInit = Date().timeIntervalSince(initializationTime)
        let mayPromptForDenied = Self.showLocationDialogWhenDeniedEnabled() && authStatus == .denied
        let mayPromptForNotDetermined = authStatus == .notDetermined
        if !isAuthorized && timeSinceInit >= 10.0 && (mayPromptForNotDetermined || mayPromptForDenied) && requestLocationPermissionEnabled() && isGUIAvailable() {
            Self.permissionPromptLock.lock()
            defer { Self.permissionPromptLock.unlock() }
            if Self.permissionAttemptCount < 1 {
                Self.permissionAttemptCount = 1
                UserDefaults.standard.set(Self.permissionAttemptCount, forKey: Self.userDefaultsPromptCountKey)
                attemptPermissionPrompt(authStatus: authStatus)
            }
        }
        if !isAuthorized && Self.permissionAttemptCount >= 1 {
            if !Self.hasLoggedAlreadyAttemptedOnce {
                Self.hasLoggedAlreadyAttemptedOnce = true
                Logger.debug("Already attempted once, skipping location permission prompt", context: "WiFiDataProvider")
            }
        } else if !isAuthorized && timeSinceInit >= 10.0 && (mayPromptForNotDetermined || mayPromptForDenied) && !requestLocationPermissionEnabled() {
            Logger.info("Location permission prompt disabled by config - skipping prompt", context: "WiFiDataProvider")
        } else if !isAuthorized && timeSinceInit >= 10.0 && (mayPromptForNotDetermined || mayPromptForDenied) && requestLocationPermissionEnabled() && !isGUIAvailable() {
            Logger.info("WiFi data request without permission in headless environment - skipping prompt", context: "WiFiDataProvider")
        } else if !isAuthorized && authStatus == .restricted {
            Logger.info("Location permission restricted by device policy - cannot prompt", context: "WiFiDataProvider")
            Logger.info("Contact your system administrator to enable location access", context: "WiFiDataProvider")
        } else if !isAuthorized && authStatus == .denied && !Self.showLocationDialogWhenDeniedEnabled() {
            Logger.debug("Location permission status denied - skipping prompt (set DD_GUI_SHOW_LOCATION_DIALOG_WHEN_DENIED=true to show dialog)", context: "WiFiDataProvider")
        } else if !isAuthorized && authStatus != .notDetermined && authStatus != .denied {
            Logger.debug("Location permission status \(authorizationStatusString()) - skipping prompt", context: "WiFiDataProvider")
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

        Logger.debug("Collected WiFi data: SSID=\(ssid.isEmpty ? "<empty>" : ssid), RSSI=\(wifiData.rssi), authorized=\(isAuthorized), permission=\(authorizationStatusString()), permission_prompt=\(requestLocationPermissionEnabled() ? "<enabled>" : "<disabled>")", context: "WiFiDataProvider")
        return wifiData
    }

    // CLLocationManagerDelegate
    // locationManagerDidChangeAuthorization: Monitors location authorization status changes.
    // Note: This is triggered when the user manually changes permission in System Settings,
    // not from programmatic permission requests (which don't work for LaunchAgent apps).
    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = getAuthorizationStatus()
        Logger.info("Location authorization changed to: \(authorizationStatusString())", context: "WiFiDataProvider")

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
