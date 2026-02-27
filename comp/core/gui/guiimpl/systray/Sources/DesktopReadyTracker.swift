import Foundation
import AppKit

/// Tracks when the desktop becomes ready (Finder launched)
/// This is the only timestamp the GUI app is responsible for - all other
/// timestamps (bootTime, loginWindowTime, loginTime) are collected by the
/// daemon via CGO to OSLogStore.
class DesktopReadyTracker {
    
    // MARK: - Desktop Ready Data
    
    /// Data structure returned via IPC - minimal, only desktop ready info
    struct DesktopReadyData: Codable {
        /// Whether the desktop is ready (Finder is running)
        let ready: Bool
        
        /// Timestamp when Finder was detected as running
        let desktopReadyTime: Double?
        
        /// Username of the logged-in user
        let username: String
        
        /// User ID
        let uid: UInt32
        
        /// Error message if any tracking failed
        let error: String?
    }
    
    // MARK: - Properties
    
    private var desktopReadyTime: Date?
    private var finderCheckTimer: Timer?
    private var isDesktopReady = false
    
    // MARK: - Initialization
    
    init() {
        Logger.info("DesktopReadyTracker initializing", context: "DesktopReadyTracker")
        setupFinderDetection()
    }
    
    // MARK: - Finder Detection
    
    private func setupFinderDetection() {
        let workspace = NSWorkspace.shared
        
        // Check if Finder is already running
        if workspace.runningApplications.contains(where: { $0.bundleIdentifier == "com.apple.finder" }) {
            markDesktopReady()
            return
        }
        
        // Listen for Finder launch
        workspace.notificationCenter.addObserver(
            self,
            selector: #selector(applicationDidLaunch(_:)),
            name: NSWorkspace.didLaunchApplicationNotification,
            object: nil
        )
        
        // Poll as backup (in case notification is missed)
        finderCheckTimer = Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { [weak self] timer in
            guard let self = self else {
                timer.invalidate()
                return
            }
            
            if NSWorkspace.shared.runningApplications.contains(where: { $0.bundleIdentifier == "com.apple.finder" }) {
                self.markDesktopReady()
                timer.invalidate()
            }
        }
        
        // Timeout after 60 seconds
        DispatchQueue.main.asyncAfter(deadline: .now() + 60) { [weak self] in
            guard let self = self, !self.isDesktopReady else { return }
            Logger.warn("Finder detection timeout after 60s - marking desktop ready", context: "DesktopReadyTracker")
            self.markDesktopReady()
        }
    }
    
    @objc private func applicationDidLaunch(_ notification: Notification) {
        guard let app = notification.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication,
              app.bundleIdentifier == "com.apple.finder" else {
            return
        }
        markDesktopReady()
    }
    
    private func markDesktopReady() {
        guard !isDesktopReady else { return }
        isDesktopReady = true
        desktopReadyTime = Date()
        finderCheckTimer?.invalidate()
        finderCheckTimer = nil
        Logger.info("Desktop ready at \(desktopReadyTime!.timeIntervalSince1970)", context: "DesktopReadyTracker")
    }
    
    // MARK: - Public API (for IPC)
    
    /// Returns desktop ready status for IPC response
    func getDesktopReadyData() -> DesktopReadyData {
        return DesktopReadyData(
            ready: isDesktopReady,
            desktopReadyTime: desktopReadyTime?.timeIntervalSince1970,
            username: NSUserName(),
            uid: getuid(),
            error: nil
        )
    }
    
    deinit {
        finderCheckTimer?.invalidate()
        NSWorkspace.shared.notificationCenter.removeObserver(self)
        Logger.info("DesktopReadyTracker deinitialized", context: "DesktopReadyTracker")
    }
}
