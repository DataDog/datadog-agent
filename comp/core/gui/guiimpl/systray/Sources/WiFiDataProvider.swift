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

    override init() {
        self.locationManager = CLLocationManager()
        super.init()
        // Keep delegate to monitor permission status changes
        self.locationManager.delegate = self
        
        let status = getAuthorizationStatus()
        NSLog("[WiFiDataProvider] Initialized with authorization status: \(authorizationStatusString())")
        
        // Log messages if permission not granted
        if status != .authorizedAlways {
            NSLog("[WiFiDataProvider] Location permission not granted - SSID/BSSID will be unavailable")
            NSLog("[WiFiDataProvider] To enable: System Settings → Privacy & Security → Location Services → Datadog Agent")
            
            // Only attempt prompt if GUI environment is available (preserves retry opportunities in headless mode)
            if isGUIAvailable() {
                NSLog("[WiFiDataProvider] GUI environment detected, attempting permission prompt...")
                attemptPermissionPrompt()
            } else {
                NSLog("[WiFiDataProvider] Headless environment detected, skipping permission prompt (will retry when GUI available)")
            }
        }
    }

    /// Check if GUI environment is available for the logged in user (can display dialogs)
    /// Returns true if osascript can execute (GUI session active), false otherwise (headless/SSH)
    private func isGUIAvailable() -> Bool {
        let task = Process()
        task.launchPath = "/usr/bin/osascript"
        task.arguments = ["-e", "tell app \"System Events\" to return name of current user"]
        task.standardOutput = Pipe()
        task.standardError = Pipe()
        
        do {
            try task.run()
            task.waitUntilExit()
            return task.terminationStatus == 0
        } catch {
            return false
        }
    }

    /// Attempt to prompt for location permission via detached process
    /// Background: LaunchAgent-launched apps cannot normally trigger prompts
    private func attemptPermissionPrompt() {
        let authStatus = getAuthorizationStatus()
        
        // Only attempt if permission not granted
        guard authStatus != .authorizedAlways else {
            return
        }
        
        // Only show once per session (prevents repeated prompts after dismissal)
        guard !sessionPromptAttempted else {
            NSLog("[WiFiDataProvider] Permission prompt already attempted this session, skipping")
            return
        }
        
        // Don't spawn duplicate if dialog process is currently running
        if let process = permissionPromptProcess, process.isRunning {
            NSLog("[WiFiDataProvider] Permission dialog process still running, skipping duplicate")
            return
        }
        
        NSLog("[WiFiDataProvider] Attempting to prompt for location permission via detached process...")
        
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
            NSLog("[WiFiDataProvider] Permission prompt process spawned (PID: \(task.processIdentifier))")
        } catch {
            NSLog("[WiFiDataProvider] Failed to spawn permission prompt: \(error)")
            permissionPromptProcess = nil
            // sessionPromptAttempted stays false - can retry later
        }
    }

    /// Get current WiFi information for the system
    func getWiFiInfo() -> WiFiData {
        // Check current authorization status (read-only, no prompt attempt)
        let authStatus = getAuthorizationStatus()
        let isAuthorized = (authStatus == .authorizedAlways)

        // One more time, on first WiFi request with no permission, try prompting
        // for location permission (if GUI available)
        if !isAuthorized && !firstWiFiRequestMade {
            if isGUIAvailable() {
                NSLog("[WiFiDataProvider] First WiFi data request without permission, attempting prompt...")
                attemptPermissionPrompt()
            } else {
                NSLog("[WiFiDataProvider] First WiFi data request without permission, but headless environment - skipping prompt")
            }
            firstWiFiRequestMade = true
        }

        // Get WiFi interface (this works regardless of location permission)
        let client = CWWiFiClient.shared()

        guard let interface = client.interface() else {
            NSLog("[WiFiDataProvider] ERROR: No WiFi interface available")
            return createErrorData(message: "No WiFi interface", authorized: isAuthorized)
        }

        // Get PHY mode string
        let phyModeStr = phyModeToString(interface.activePHYMode())

        // Check if interface is active
        if phyModeStr == "None" {
            NSLog("[WiFiDataProvider] WiFi interface is not active (PHY mode: None)")
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
            NSLog("[WiFiDataProvider] WARN: SSID/BSSID empty - location permission not granted")
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

        NSLog("[WiFiDataProvider] Collected WiFi data: SSID=\(ssid.isEmpty ? "<empty>" : ssid), RSSI=\(wifiData.rssi), authorized=\(isAuthorized)")
        return wifiData
    }

    // CLLocationManagerDelegate
    // locationManagerDidChangeAuthorization: Monitors location authorization status changes.
    // Note: This is triggered when the user manually changes permission in System Settings,
    // not from programmatic permission requests (which don't work for LaunchAgent apps).
    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = getAuthorizationStatus()
        NSLog("[WiFiDataProvider] Location authorization changed to: \(authorizationStatusString())")
        
        // Reset flags when authorization status changes
        // This allows showing the prompt again if permission is revoked or status changes
        sessionPromptAttempted = false
        permissionPromptProcess = nil
        firstWiFiRequestMade = false

        switch status {
        case .authorizedAlways:
            NSLog("[WiFiDataProvider] Location permission GRANTED - WiFi SSID/BSSID will be available")
        case .denied:
            NSLog("[WiFiDataProvider] Location permission DENIED - WiFi SSID/BSSID will be unavailable")
        case .restricted:
            NSLog("[WiFiDataProvider] Location permission RESTRICTED")
        case .notDetermined:
            NSLog("[WiFiDataProvider] Location permission NOT DETERMINED")
        @unknown default:
            NSLog("[WiFiDataProvider] Unknown authorization status: \(status.rawValue)")
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
