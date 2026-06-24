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
/// When authorization is `.notDetermined`, the system location prompt is requested via
/// `requestWhenInUseAuthorization()` (gated by the WLAN check flag on each IPC request).
class WiFiDataProvider: NSObject, CLLocationManagerDelegate {
    // Note: LaunchAgent-launched apps cannot trigger location permission prompts
    // on macOS (prompts are auto-denied by the system). Permission must be granted
    // manually via System Settings -> Privacy & Security -> Location Services.
    // During the launch time of this GUI app, we will attempt to prompt for permission
    // based on the availability of the GUI environment.
    private let locationManager: CLLocationManager
    private let initializationTime: Date = Date()  // Track when WiFiDataProvider was initialized

    override init() {
        self.locationManager = CLLocationManager()
        super.init()
        // Keep delegate to monitor permission status changes
        self.locationManager.delegate = self
    }

    /// Check if GUI environment is available (can display dialogs)
    /// Returns true if running in an Aqua (GUI) session, false for Background/SSH/daemon sessions
    /// Uses launchctl to detect an Aqua (interactive GUI) session.
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

            // Only "Aqua" indicates a GUI session where dialogs can be displayed
            // Other values: "Background" (SSH/daemon), "LoginWindow" (login screen)
            return output == "Aqua"

        } catch {
            Logger.error("Failed to check session type: \(error)", context: "WiFiDataProvider")
            return false
        }
    }

    /// Request the system location authorization prompt when status is `.notDetermined`.
    /// TCC updates authorization after the user responds; until then the status stays `.notDetermined`.
    private func attemptPermissionPrompt(authStatus: CLAuthorizationStatus) {
        if authStatus == .restricted {
            Logger.info("Location permission restricted by policy; cannot prompt", context: "WiFiDataProvider")
            return
        }

        guard authStatus == .notDetermined else {
            return
        }

        // Core Location expects authorization requests on the main thread; getWiFiInfo is invoked from
        // WiFiIPCServer.handleClient on DispatchQueue.global(qos: .background) when the agent asks for WiFi data.
        DispatchQueue.main.async { [weak self] in
            // If the provider was deallocated before this runs, skip (weak avoids retaining self via the async closure).
            guard let self = self else { return }
            if #available(macOS 10.15, *) {
                self.locationManager.requestWhenInUseAuthorization()
            }
        }
    }

    /// Get current WiFi information for the system. requestLocationPermission
    /// comes from the caller (the WLAN check, via IPC) so we never need to
    /// query the agent ourselves.
    func getWiFiInfo(requestLocationPermission: Bool) -> WiFiData {
        // Check current authorization status (read-only, no prompt attempt)
        let authStatus = getAuthorizationStatus()
        let isAuthorized = (authStatus == .authorizedAlways)

        // Permission detection during WLAN data collection
        // Check permission at least 10 seconds after initialization to avoid TCC loading race condition.
        // Only `.notDetermined` triggers the native prompt; after Allow/Deny, authorization status reflects TCC and this block stops running.
        let timeSinceInit = Date().timeIntervalSince(initializationTime)
        if authStatus == .notDetermined && timeSinceInit >= 10.0 {
            if requestLocationPermission && isGUIAvailable() {
                attemptPermissionPrompt(authStatus: authStatus)
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

        Logger.debug("Collected WiFi data: SSID=\(ssid.isEmpty ? "<empty>" : ssid), RSSI=\(wifiData.rssi), locationAuthorized=\(isAuthorized)", context: "WiFiDataProvider")
        return wifiData
    }

    // CLLocationManagerDelegate
    // locationManagerDidChangeAuthorization: Monitors location authorization status changes.
    // Note: This is triggered when the user manually changes permission in System Settings,
    // not from programmatic permission requests (which don't work for LaunchAgent apps).
    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = getAuthorizationStatus()
        switch status {
        case .authorizedAlways, .denied, .restricted, .notDetermined:
            Logger.info("Location authorization: \(authorizationStatusString())", context: "WiFiDataProvider")
        @unknown default:
            Logger.error("Unknown location authorization status: \(status.rawValue)", context: "WiFiDataProvider")
        }
    }

    // Authorization Status Helper
    private func getAuthorizationStatus() -> CLAuthorizationStatus {
        return locationManager.authorizationStatus
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
