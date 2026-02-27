# macOS Boot and Logon Duration Collection - Design Document

## Overview

This document describes the design and implementation plan for collecting boot duration and logon duration data on macOS. The implementation uses a **daemon-driven architecture** where:

- **Daemon (Go + CGO)**: Collects boot time, login window time, and login time via sysctl and Unified Logging (OSLogStore)
- **GUI App (Swift)**: Only detects when the desktop is ready (Finder launched) and reports via IPC

### Goals

1. **Boot Duration**: Measure the time from system boot (`bootTime` via sysctl) to login window appearing (`loginWindowTime` via Unified Logging)
2. **Logon Duration**: Measure the time from user entering credentials (`loginTime` via Unified Logging) to desktop ready (`desktopReadyTime` via GUI)
3. **High Accuracy**: Use native macOS APIs (Unified Logging via CGO) for timing precision
4. **Minimal IPC**: Only query GUI for desktop ready state; all other timestamps from daemon
5. **Graceful Degradation**: Report partial data if GUI is unavailable (boot/login times still available)
6. **One-Time Collection**: Collect data once per boot/login session (not repeated polling)

### Data to be Collected (PoC - Logged Only)

For this proof-of-concept, data is logged to the agent log file rather than sent as metrics or events. Future iterations may send this as Datadog Events.

| Field | Description | Source |
|-------|-------------|--------|
| `boot_timestamp` | Unix timestamp of system boot | Daemon: `sysctl kern.boottime` |
| `login_window_timestamp` | When login window appeared | Daemon: CGO to OSLogStore |
| `boot_duration_seconds` | `loginWindowTime` - `bootTime` | Daemon: calculated |
| `login_timestamp` | When user entered credentials | Daemon: CGO to OSLogStore (`com.apple.sessionDidLogin`) |
| `desktop_ready_timestamp` | When Finder launched | GUI: NSWorkspace (via IPC) |
| `logon_duration_seconds` | `desktopReadyTime` - `loginTime` | Daemon: calculated |
| `filevault_enabled` | Whether FileVault is enabled | Daemon: CGO to `fdesetup` |
| `username`, `uid` | Logged-in user info | GUI: via IPC |

---

## Background

### Why Split Architecture?

macOS presents unique challenges for boot/logon duration collection:

1. **LaunchDaemon (Main Agent)**: Runs at boot as root, before any user logs in
   - ✅ Can get boot time via `sysctl kern.boottime`
   - ✅ Can query Unified Logging for `loginWindowTime` and `loginTime` (via CGO to OSLogStore)
   - ❌ Cannot detect Finder launch (no GUI access, no NSWorkspace)

2. **LaunchAgent (GUI App)**: Runs at user login in user context
   - ✅ Can detect Finder launch via NSWorkspace (for `desktopReadyTime`)
   - ❌ Starts *during* login, may miss early events

**Solution**: 
- Daemon queries Unified Logging directly via CGO for login timestamps
- GUI app only reports desktop ready state (Finder detection) via IPC

### Why CGO for Unified Logging?

The OSLogStore API is only available in Objective-C/Swift. Options considered:

| Approach | Pros | Cons |
|----------|------|------|
| **CGO to OSLogStore** | Direct API access, no subprocess overhead, type-safe | Requires Objective-C bridge file |
| Shell out to `log show` | No CGO needed | Subprocess overhead, parsing text output, security concerns |
| Keep in GUI (Swift) | Pure Swift implementation | Requires IPC for timestamps, GUI must be running |

**Decision**: CGO to OSLogStore provides the best balance of performance, reliability, and security.

#### CGO Requirements

- macOS 10.15+ (Catalina) for OSLogStore API
- Frameworks: `Foundation`, `OSLog`
- Build with CGO enabled (`CGO_ENABLED=1`)
- The daemon runs as root, which has full access to local logs

### Design Approach: Daemon-Driven with GUI Ready Flag

The daemon handles most of the timestamp collection directly:

1. **Daemon gets timestamps directly**:
   - `bootTime` via `sysctl kern.boottime`
   - `loginWindowTime` via Unified Logging (CGO to OSLogStore)
   - `loginTime` via Unified Logging (CGO to OSLogStore)

2. **GUI only reports desktop ready state**:
   - GUI detects Finder launch via NSWorkspace
   - GUI sets `ready` flag and `desktopReadyTime` when Finder is running
   - Agent queries GUI for this information via IPC

3. **One-time collection**:
   - Agent queries GUI for `ready` flag
   - If `ready == false`, agent retries after 5 seconds
   - If `ready == true`, agent logs all data **once** and stops until next boot
   - Agent tracks boot time to detect reboots and reset collection state

This minimizes IPC (only for Finder detection) while leveraging the daemon's root access for log queries.

### Existing Infrastructure

The Datadog Agent already has this pattern implemented for WiFi metrics collection:

- **GUI Swift App**: `comp/core/gui/guiimpl/systray/Sources/`
  - Runs as LaunchAgent at user login
  - Creates Unix socket at `<install_dir>/run/ipc/gui-<uid>.sock`
  - Responds to JSON commands (e.g., `get_wifi_info`)

- **Main Agent**: `pkg/collector/corechecks/net/wlan/wlan_darwin.go`
  - Connects to GUI socket
  - Sends JSON request, receives JSON response
  - Validates socket ownership for security

This design extends the existing IPC mechanism to support login event queries, with the addition of the "ready" flag pattern.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                     macOS Boot/Logon Duration (Daemon-Driven)                    │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                   │
│  ┌─────────────────────────────────────────┐                                     │
│  │  LaunchDaemon (Main Agent)              │                                     │
│  │  /Library/LaunchDaemons/                │                                     │
│  │  com.datadoghq.agent                    │                                     │
│  │                                         │                                     │
│  │  ┌─────────────────────────────────┐   │                                     │
│  │  │  bootlogon check (Go + CGO)     │   │      IPC Request                    │
│  │  │                                 │   │   {"command":"get_desktop_ready"}   │
│  │  │  • kern.boottime (sysctl)       │───┼────────────────────────────────►    │
│  │  │  • loginWindowTime (OSLogStore) │   │                                     │
│  │  │  • loginTime (OSLogStore via CGO)   │                                     │
│  │  │  • Query GUI for desktop ready  │◄──┼────────────────────────────────     │
│  │  │  • If ready: log data ONCE      │   │    IPC Response (ready + time)      │
│  │  │  • If not ready: retry in 5s    │   │                                     │
│  │  └─────────────────────────────────┘   │                                     │
│  └─────────────────────────────────────────┘                                     │
│                         ▲                                                        │
│                         │  Shared IPC Directory                                  │
│                         │  /opt/datadog-agent/run/ipc/                          │
│                         │                                                        │
│  ┌─────────────────────────────────────────┐                                     │
│  │  LaunchAgent (GUI App - Swift)          │                                     │
│  │  /Library/LaunchAgents/                 │                                     │
│  │  com.datadoghq.gui                      │                                     │
│  │                                         │                                     │
│  │  ┌─────────────────────────────────┐   │     Unix Socket                     │
│  │  │  DesktopReadyTracker.swift      │   │  gui-<uid>.sock                     │
│  │  │                                 │   │◄─────────────────────────────►      │
│  │  │  • NSWorkspace notifications    │   │                                     │
│  │  │  • Finder launch detection      │   │                                     │
│  │  │  • "ready" flag                 │   │                                     │
│  │  │  • desktopReadyTime             │   │                                     │
│  │  └─────────────────────────────────┘   │                                     │
│  └─────────────────────────────────────────┘                                     │
│                                                                                   │
└──────────────────────────────────────────────────────────────────────────────────┘
```

### Timing Diagram

```
     POWER ON
         │
         ▼
    ┌─────────┐
    │ Firmware│  (Not measurable - before OS)
    │  POST   │
    └────┬────┘
         │
         ▼
    ┌─────────┐
    │  Boot   │  ◄── kern.boottime (sysctl) - THIS IS OUR REFERENCE POINT
    │  Loader │      bootTime
    └────┬────┘
         │
         ▼                                    ┌─────────────────────────────────────┐
    ┌─────────┐                               │         BOOT DURATION               │
    │ Kernel  │                               │   (bootTime → loginWindowTime)      │
    │  Init   │                               │                                     │
    └────┬────┘                               │  Measures: system initialization    │
         │                                    │  from kernel boot to login window   │
         ▼                                    │  appearing (user can enter creds)   │
    ┌─────────┐                               └─────────────────────────────────────┘
    │ launchd │  ◄── LaunchDaemon starts
    │  start  │      (main agent)
    └────┬────┘
         │
         ▼
    ┌─────────┐
    │ Login   │  ◄── LoginWindow appears - loginWindowTime
    │ Window  │      (from Unified Logging)
    └────┬────┘      
         │
         ▼
    ┌─────────┐
    │  User   │  ◄── User enters credentials - loginTime
    │  Auth   │      (from Unified Logging: com.apple.sessionDidLogin)
    └────┬────┘
         │                                    ┌─────────────────────────────────────┐
         ▼                                    │         LOGON DURATION              │
    ┌─────────┐                               │   (loginTime → desktopReadyTime)    │
    │FileVault│  (If enabled - adds time)    │                                     │
    │ Unlock  │                               │  Measures: user login experience    │
    └────┬────┘                               │  from entering credentials to       │
         │                                    │  usable desktop                     │
         ▼                                    └─────────────────────────────────────┘
    ┌─────────┐
    │ Finder  │  ◄── desktopReadyTime
    │ Launch  │
    └────┬────┘
         │
         ▼
    ┌─────────┐
    │ Desktop │  ◄── User can start working
    │  Ready  │
    └─────────┘
```

---

## Implementation Details

### File Structure

The implementation uses the component architecture (not a check), split between:
1. **Logon Duration Component** - manages the lifecycle and event submission
2. **System-Probe Module** - handles privileged OSLogStore queries
3. **pkg/bootlogon** - shared data types and CGO code
4. **GUI App** - detects desktop ready state

```
# Logon Duration Component (agent - manages lifecycle and event submission)
comp/logonduration/
├── def/component.go          # Component interface definition
├── impl/
│   ├── impl.go               # Windows implementation
│   ├── impl_darwin.go        # macOS implementation (NEW)
│   ├── analyzer.go           # Windows ETL analyzer
│   └── autologger.go         # Windows autologger management
└── fx/
    ├── fx.go                 # Windows fx wiring
    ├── fx_darwin.go          # macOS fx wiring (NEW)
    └── fx_noop.go            # Noop for other platforms (NEW)

# System-Probe (runs as root - handles OSLogStore queries)
pkg/bootlogon/
├── timestamps.go             # Data structures for login timestamps and GUI IPC
├── timestamps_darwin.go      # macOS-specific: CGO to OSLogStore + GUI IPC
├── timestamps_darwin.m       # Objective-C bridge for OSLogStore
└── timestamps_noop.go        # Stub for non-darwin platforms

cmd/system-probe/modules/
└── boot_logon_darwin.go      # System-probe module exposing /check endpoint

pkg/system-probe/config/config.go  # Contains BootLogonModule constant

# GUI App (user context - detects desktop ready state)
comp/core/gui/guiimpl/systray/Sources/
├── gui.swift                 # (Modify) Initialize DesktopReadyTracker
├── main.swift                # (No changes needed)
├── DesktopReadyTracker.swift # (New) Finder launch detection only
├── WiFiIPCServer.swift       # (Modify) Add get_desktop_ready command handler
├── WiFiDataProvider.swift    # (No changes needed)
└── Logger.swift              # (No changes needed)
```

### Architecture: Component + System-Probe

The implementation uses a component-based architecture (like software inventory) rather than a check:

1. **Logon Duration Component** (`comp/logonduration/impl/impl_darwin.go`):
   - Runs on agent startup via fx lifecycle hooks
   - Detects reboot via persistent cache
   - Queries system-probe for login timestamps (requires root)
   - Queries GUI app for desktop ready state (via IPC)
   - Submits Event Management v2 event to Datadog

2. **System-Probe Module** (`cmd/system-probe/modules/boot_logon_darwin.go`):
   - Runs as root
   - Uses CGO to query OSLogStore for login timestamps
   - Exposes `/check` HTTP endpoint

3. **GUI App**:
   - Detects when Finder launches (desktop ready)
   - Responds to IPC queries from the component

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Boot/Logon Duration Flow                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────────────────────────────┐                                   │
│   │  Logon Duration Component           │                                   │
│   │  comp/logonduration/impl/           │                                   │
│   │                                     │                                   │
│   │  • Detects reboot (persistent cache)│                                   │
│   │  • GetBootTime() via sysctl         │ ──── No root needed              │
│   │  • GetDesktopReadyData() via IPC    │ ──── No root needed              │
│   │  • Queries system-probe for login   │ ──── Queries system-probe        │
│   │  • Submits Event Management event   │                                   │
│   └─────────────────┬───────────────────┘                                   │
│                     │                                                       │
│          HTTP GET /modules/boot_logon/check                                 │
│                     │                                                       │
│                     ▼                                                       │
│   ┌─────────────────────────────────────┐                                   │
│   │  System-Probe Module (boot_logon)   │                                   │
│   │  cmd/system-probe/modules/          │                                   │
│   │                                     │                                   │
│   │  • Runs as root                     │                                   │
│   │  • CGO to OSLogStore                │                                   │
│   │  • GetLoginTimestamps()             │                                   │
│   │    - loginWindowTime                │                                   │
│   │    - loginTime                      │                                   │
│   │    - fileVaultEnabled               │                                   │
│   └─────────────────────────────────────┘                                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Part 1: GUI Agent (Swift) - Desktop Ready Detection Only

The GUI app's only responsibility is detecting when the desktop is ready (Finder launched). All other timestamps are collected by the daemon via CGO.

### 1.1 New File: `DesktopReadyTracker.swift`

Create a simple Swift class to detect Finder launch.

**Location**: `comp/core/gui/guiimpl/systray/Sources/DesktopReadyTracker.swift`

```swift
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
```

### 1.2 Modify: `gui.swift`

Add DesktopReadyTracker initialization to the AgentGUI class.

**Location**: `comp/core/gui/guiimpl/systray/Sources/gui.swift`

**Changes**:

```swift
// In AgentGUI class, add property:
var desktopReadyTracker: DesktopReadyTracker?

// In init(), after WiFi initialization, add:
desktopReadyTracker = DesktopReadyTracker()
Logger.info("Desktop ready tracker initialized", context: "AgentGUI")
```

### 1.3 Modify: `WiFiIPCServer.swift`

Extend the IPC server to handle the `get_desktop_ready` command.

**Location**: `comp/core/gui/guiimpl/systray/Sources/WiFiIPCServer.swift`

**Changes**:

```swift
// Add new response structure (near top of file):

/// Response structure for desktop ready IPC
struct DesktopReadyIPCResponse: Codable {
    let success: Bool
    let data: DesktopReadyTracker.DesktopReadyData?
    let error: String?
}

// Modify init to accept DesktopReadyTracker:

private weak var desktopReadyTracker: DesktopReadyTracker?

init(wifiDataProvider: WiFiDataProvider, desktopReadyTracker: DesktopReadyTracker?) {
    self.wifiDataProvider = wifiDataProvider
    self.desktopReadyTracker = desktopReadyTracker
    // ... rest of init
}

// In handleClient() method, add case in switch statement:

case "get_desktop_ready":
    if let tracker = desktopReadyTracker {
        let data = tracker.getDesktopReadyData()
        let response = DesktopReadyIPCResponse(success: true, data: data, error: nil)
        sendDesktopReadyResponse(clientFD, response: response)
    } else {
        let response = DesktopReadyIPCResponse(success: false, data: nil, error: "Desktop ready tracker not initialized")
        sendDesktopReadyResponse(clientFD, response: response)
    }

// Add helper method for sending responses:

private func sendDesktopReadyResponse(_ clientFD: Int32, response: DesktopReadyIPCResponse) {
    guard let responseData = try? JSONEncoder().encode(response) else {
        Logger.error("Failed to encode desktop ready response", context: "WiFiIPCServer")
        return
    }
    
    var data = responseData
    data.append(contentsOf: [UInt8(ascii: "\n")])
    
    data.withUnsafeBytes { ptr in
        guard let bytesPtr = ptr.baseAddress?.assumingMemoryBound(to: UInt8.self) else { return }
        let bytesWritten = write(clientFD, bytesPtr, data.count)
        if bytesWritten < 0 && errno != EPIPE {
            Logger.error("Write failed: \(String(cString: strerror(errno)))", context: "WiFiIPCServer")
        }
    }
}
```

Update the initialization in `gui.swift` to pass DesktopReadyTracker:

```swift
// In AgentGUI.init(), update WiFiIPCServer initialization:
if let provider = wifiDataProvider {
    wifiIPCServer = WiFiIPCServer(wifiDataProvider: provider, desktopReadyTracker: desktopReadyTracker)
}
```

---

## Part 2: Main Agent Check (Go + CGO)

The daemon is responsible for:
1. Getting `bootTime` via sysctl
2. Getting `loginWindowTime` and `loginTime` via CGO to OSLogStore
3. Getting `desktopReadyTime` from the GUI via IPC
4. Calculating and logging durations

### 2.1 New File: `bootlogon.go`

Main check implementation. The daemon queries login timestamps directly via CGO, and only uses IPC to get desktop ready status from the GUI.

**Location**: `pkg/collector/corechecks/system/bootlogon/bootlogon.go`

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootlogon implements the boot and logon duration check for macOS.
// This is a PoC that logs boot/logon duration data rather than sending metrics.
package bootlogon

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const CheckName = "bootlogon"

// Check collects boot and logon duration data (PoC - logs only)
type Check struct {
	core.CheckBase

	// Cached timestamps (don't change after collection)
	bootTime          time.Time
	loginWindowTime   *time.Time
	loginTime         *time.Time
	timestampsCached  bool

	// Track the boot time we collected data for, to detect reboots
	collectedForBootTime time.Time
	// Whether we've already logged data for this boot session
	dataCollected bool
}

// Run executes the check
func (c *Check) Run() error {
	// Get boot time and login timestamps (cached after first successful call)
	if !c.timestampsCached {
		if err := c.cacheTimestamps(); err != nil {
			log.Warnf("bootlogon: failed to cache timestamps: %v", err)
			return nil
		}
	}

	// Check if system rebooted since we last collected data
	if c.dataCollected && !c.collectedForBootTime.Equal(c.bootTime) {
		log.Infof("bootlogon: detected reboot (previous boot: %v, current: %v), resetting collection state",
			c.collectedForBootTime, c.bootTime)
		c.dataCollected = false
		c.timestampsCached = false
	}

	// If we've already collected data for this boot session, do nothing
	if c.dataCollected {
		log.Debugf("bootlogon: data already collected for this boot session, skipping")
		return nil
	}

	// Get desktop ready status from GUI (only thing we need from GUI)
	desktopData, err := GetDesktopReadyData()
	if err != nil {
		log.Debugf("bootlogon: failed to get desktop ready data from GUI: %v", err)
		// Not a fatal error - GUI might not be running (SSH session, headless, etc.)
		return nil
	}

	// Check if desktop is ready
	if !desktopData.Ready {
		log.Debugf("bootlogon: desktop not ready yet, will retry on next check interval")
		return nil
	}

	// Desktop is ready - log the data (PoC: just logging, not sending metrics/events)
	c.logBootLogonData(desktopData)

	// Mark as collected for this boot session
	c.dataCollected = true
	c.collectedForBootTime = c.bootTime

	return nil
}

// cacheTimestamps retrieves and caches boot time and login timestamps
func (c *Check) cacheTimestamps() error {
	// Get boot time via sysctl
	bt, err := GetBootTime()
	if err != nil {
		return err
	}
	c.bootTime = bt
	log.Infof("bootlogon: boot time: %v", c.bootTime)

	// Get login window time via CGO to OSLogStore
	if lwt, err := GetLoginWindowTime(c.bootTime); err == nil {
		c.loginWindowTime = &lwt
		log.Infof("bootlogon: login window time: %v", lwt)
	} else {
		log.Warnf("bootlogon: failed to get login window time: %v", err)
	}

	// Get login time via CGO to OSLogStore
	if lt, err := GetLoginTime(c.bootTime); err == nil {
		c.loginTime = &lt
		log.Infof("bootlogon: login time: %v", lt)
	} else {
		log.Warnf("bootlogon: failed to get login time: %v", err)
	}

	c.timestampsCached = true
	return nil
}

// logBootLogonData logs the collected boot/logon data (PoC implementation)
func (c *Check) logBootLogonData(desktopData *DesktopReadyData) {
	// Build tags for logging
	filevaultTag := "unknown"
	if fv, err := IsFileVaultEnabled(); err == nil {
		if fv {
			filevaultTag = "enabled"
		} else {
			filevaultTag = "disabled"
		}
	}

	// Calculate durations
	// Boot Duration: bootTime -> loginWindowTime (system initialization until login window appears)
	// Logon Duration: loginTime -> desktopReadyTime (user enters credentials until desktop is ready)
	var bootDuration, logonDuration float64

	// Boot Duration = loginWindowTime - bootTime
	if c.loginWindowTime != nil {
		bootDuration = c.loginWindowTime.Sub(c.bootTime).Seconds()
	}

	// Logon Duration = desktopReadyTime - loginTime
	if c.loginTime != nil && desktopData.DesktopReadyTime != nil {
		desktopReady := time.Unix(0, int64(*desktopData.DesktopReadyTime*float64(time.Second)))
		logonDuration = desktopReady.Sub(*c.loginTime).Seconds()
	}

	// Log all the data (PoC - future: send as Datadog Event)
	log.Infof("bootlogon: ========== Boot/Logon Duration Data ==========")
	log.Infof("bootlogon: username=%s, uid=%d, filevault=%s", desktopData.Username, desktopData.UID, filevaultTag)
	log.Infof("bootlogon: boot_timestamp=%.3f", float64(c.bootTime.Unix()))
	if c.loginWindowTime != nil {
		log.Infof("bootlogon: login_window_timestamp=%.3f", float64(c.loginWindowTime.Unix()))
	}
	if c.loginTime != nil {
		log.Infof("bootlogon: login_timestamp=%.3f", float64(c.loginTime.Unix()))
	}
	if desktopData.DesktopReadyTime != nil {
		log.Infof("bootlogon: desktop_ready_timestamp=%.3f", *desktopData.DesktopReadyTime)
	}

	// Log calculated durations with sanity checks
	if bootDuration > 0 && bootDuration < 3600 {
		log.Infof("bootlogon: boot_duration_seconds=%.2f", bootDuration)
	} else if bootDuration != 0 {
		log.Warnf("bootlogon: boot_duration out of range: %.2f seconds", bootDuration)
	}

	if logonDuration > 0 && logonDuration < 600 {
		log.Infof("bootlogon: logon_duration_seconds=%.2f", logonDuration)
	} else if logonDuration != 0 {
		log.Warnf("bootlogon: logon_duration out of range: %.2f seconds", logonDuration)
	}

	log.Infof("bootlogon: ================================================")
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
	}
}
```

### 2.2 New File: `bootlogon_darwin.go`

macOS-specific implementation with CGO for OSLogStore, sysctl, and IPC client.

**Location**: `pkg/collector/corechecks/system/bootlogon/bootlogon_darwin.go`

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package bootlogon

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework OSLog

#include <stdlib.h>

// Returns Unix timestamp (seconds since epoch) or 0 on error
// Query type: 0 = login window time, 1 = login time (sessionDidLogin)
double queryLoginTimestamp(double bootTimestamp, int queryType);

// Returns 1 if FileVault is enabled, 0 if disabled, -1 on error
int checkFileVaultEnabled();
*/
import "C"

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxIPCResponseSize = 4096
	ipcTimeout         = 2 * time.Second
)

// DesktopReadyData mirrors the Swift DesktopReadyTracker.DesktopReadyData struct
type DesktopReadyData struct {
	Ready            bool     `json:"ready"`
	DesktopReadyTime *float64 `json:"desktopReadyTime"`
	Username         string   `json:"username"`
	UID              uint32   `json:"uid"`
	Error            *string  `json:"error"`
}

// DesktopReadyIPCResponse represents the IPC response from GUI
type DesktopReadyIPCResponse struct {
	Success bool              `json:"success"`
	Data    *DesktopReadyData `json:"data"`
	Error   *string           `json:"error"`
}

// GetBootTime returns the system boot time using sysctl kern.boottime
func GetBootTime() (time.Time, error) {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return time.Time{}, fmt.Errorf("sysctl kern.boottime failed: %w", err)
	}
	return time.Unix(tv.Sec, int64(tv.Usec)*1000), nil
}

// GetLoginWindowTime queries OSLogStore for when the login window appeared
func GetLoginWindowTime(bootTime time.Time) (time.Time, error) {
	bootTimestamp := C.double(float64(bootTime.Unix()))
	result := C.queryLoginTimestamp(bootTimestamp, 0) // 0 = login window time
	
	if result == 0 {
		return time.Time{}, fmt.Errorf("failed to query login window time from unified logs")
	}
	
	return time.Unix(int64(result), int64((result-float64(int64(result)))*1e9)), nil
}

// GetLoginTime queries OSLogStore for when the user entered credentials
func GetLoginTime(bootTime time.Time) (time.Time, error) {
	bootTimestamp := C.double(float64(bootTime.Unix()))
	result := C.queryLoginTimestamp(bootTimestamp, 1) // 1 = login time (sessionDidLogin)
	
	if result == 0 {
		return time.Time{}, fmt.Errorf("failed to query login time from unified logs")
	}
	
	return time.Unix(int64(result), int64((result-float64(int64(result)))*1e9)), nil
}

// IsFileVaultEnabled checks if FileVault is enabled
func IsFileVaultEnabled() (bool, error) {
	result := C.checkFileVaultEnabled()
	if result < 0 {
		return false, fmt.Errorf("failed to check FileVault status")
	}
	return result == 1, nil
}

// GetDesktopReadyData retrieves desktop ready status from the GUI via IPC
func GetDesktopReadyData() (*DesktopReadyData, error) {
	uid, err := getConsoleUserUID()
	if err != nil {
		return nil, fmt.Errorf("no console user: %w", err)
	}

	socketPath := filepath.Join(pkgconfigsetup.InstallPath, "run", "ipc", fmt.Sprintf("gui-%s.sock", uid))

	if err := validateSocketOwnership(socketPath, uid); err != nil {
		return nil, fmt.Errorf("socket validation failed: %w", err)
	}

	return fetchDesktopReadyFromGUI(socketPath, ipcTimeout)
}

func getConsoleUserUID() (string, error) {
	cmd := exec.Command("/usr/bin/stat", "-f", "%u", "/dev/console")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to stat /dev/console: %w", err)
	}

	uid := strings.TrimSpace(string(output))
	if uid == "" || uid == "0" {
		return "", fmt.Errorf("no console user logged in (UID: %s)", uid)
	}

	log.Debugf("bootlogon: console user UID: %s", uid)
	return uid, nil
}

func validateSocketOwnership(socketPath string, expectedUID string) error {
	fileInfo, err := os.Stat(socketPath)
	if err != nil {
		return fmt.Errorf("cannot stat socket %s: %w", socketPath, err)
	}

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot get socket file stat")
	}

	actualUID := strconv.FormatUint(uint64(stat.Uid), 10)
	if actualUID != expectedUID {
		return fmt.Errorf("socket owner mismatch: expected UID %s, got UID %s", expectedUID, actualUID)
	}

	return nil
}

func fetchDesktopReadyFromGUI(socketPath string, timeout time.Duration) (*DesktopReadyData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to GUI socket: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Send request for desktop ready status only
	request := map[string]string{"command": "get_desktop_ready"}
	requestData, _ := json.Marshal(request)
	conn.Write(append(requestData, '\n'))

	reader := bufio.NewReaderSize(conn, maxIPCResponseSize)
	responseLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response DesktopReadyIPCResponse
	if err := json.Unmarshal([]byte(responseLine), &response); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}

	if !response.Success || response.Data == nil {
		errMsg := "unknown error"
		if response.Error != nil {
			errMsg = *response.Error
		}
		return nil, fmt.Errorf("GUI returned error: %s", errMsg)
	}

	return response.Data, nil
}
```

### 2.3 New File: `bootlogon_darwin.m`

Objective-C implementation for OSLogStore queries via CGO.

**Location**: `pkg/collector/corechecks/system/bootlogon/bootlogon_darwin.m`

```objc
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#import <Foundation/Foundation.h>
#import <OSLog/OSLog.h>

// Query type constants
#define QUERY_LOGIN_WINDOW_TIME 0
#define QUERY_LOGIN_TIME 1

// queryLoginTimestamp queries OSLogStore for login-related timestamps
// Returns Unix timestamp (seconds since epoch) or 0 on error
// queryType: 0 = login window time (SessionAgentNotificationCenter)
//            1 = login time (com.apple.sessionDidLogin)
double queryLoginTimestamp(double bootTimestamp, int queryType) {
    @autoreleasepool {
        NSError *error = nil;
        
        // Get the local log store (requires admin privileges, non-sandboxed)
        OSLogStore *logStore = [OSLogStore localStoreAndReturnError:&error];
        if (!logStore) {
            NSLog(@"bootlogon: Failed to open local log store: %@", error);
            return 0;
        }
        
        // Create position from boot time
        NSDate *bootDate = [NSDate dateWithTimeIntervalSince1970:bootTimestamp];
        OSLogPosition *position = [logStore positionWithDate:bootDate];
        
        // Build predicate based on query type
        NSPredicate *predicate;
        if (queryType == QUERY_LOGIN_WINDOW_TIME) {
            // Look for first loginwindow SessionAgentNotificationCenter message
            predicate = [NSPredicate predicateWithFormat:
                @"process == 'loginwindow' AND composedMessage CONTAINS 'SessionAgentNotificationCenter'"];
        } else {
            // Look for sessionDidLogin message (true login event)
            predicate = [NSPredicate predicateWithFormat:
                @"process == 'loginwindow' AND composedMessage CONTAINS 'com.apple.sessionDidLogin'"];
        }
        
        // Query the log store
        OSLogEnumerator *enumerator = [logStore entriesEnumeratorWithOptions:0
                                                                   position:position
                                                                  predicate:predicate
                                                                      error:&error];
        if (!enumerator) {
            NSLog(@"bootlogon: Failed to create log enumerator: %@", error);
            return 0;
        }
        
        // Get the first matching entry
        OSLogEntryLog *entry = [enumerator nextObject];
        if (entry) {
            return [entry.date timeIntervalSince1970];
        }
        
        NSLog(@"bootlogon: No matching log entry found for query type %d", queryType);
        return 0;
    }
}

// checkFileVaultEnabled checks if FileVault is enabled
// Returns 1 if enabled, 0 if disabled, -1 on error
int checkFileVaultEnabled(void) {
    @autoreleasepool {
        NSTask *task = [[NSTask alloc] init];
        task.launchPath = @"/usr/bin/fdesetup";
        task.arguments = @[@"status"];
        
        NSPipe *pipe = [NSPipe pipe];
        task.standardOutput = pipe;
        task.standardError = [NSFileHandle fileHandleWithNullDevice];
        
        NSError *error = nil;
        if (![task launchAndReturnError:&error]) {
            NSLog(@"bootlogon: Failed to run fdesetup: %@", error);
            return -1;
        }
        
        [task waitUntilExit];
        
        NSData *data = [[pipe fileHandleForReading] readDataToEndOfFile];
        NSString *output = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        
        if ([output containsString:@"FileVault is On"]) {
            return 1;
        }
        return 0;
    }
}
```

### 2.4 New File: `bootlogon_noop.go`

Stub for non-darwin platforms.

**Location**: `pkg/collector/corechecks/system/bootlogon/bootlogon_noop.go`

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package bootlogon

import (
	"errors"
	"time"
)

// DesktopReadyData placeholder for non-darwin platforms
type DesktopReadyData struct {
	Ready            bool
	DesktopReadyTime *float64
	Username         string
	UID              uint32
	Error            *string
}

// GetBootTime is not implemented on this platform
func GetBootTime() (time.Time, error) {
	return time.Time{}, errors.New("bootlogon: not implemented on this platform")
}

// GetLoginWindowTime is not implemented on this platform
func GetLoginWindowTime(bootTime time.Time) (time.Time, error) {
	return time.Time{}, errors.New("bootlogon: not implemented on this platform")
}

// GetLoginTime is not implemented on this platform
func GetLoginTime(bootTime time.Time) (time.Time, error) {
	return time.Time{}, errors.New("bootlogon: not implemented on this platform")
}

// IsFileVaultEnabled is not implemented on this platform
func IsFileVaultEnabled() (bool, error) {
	return false, errors.New("bootlogon: not implemented on this platform")
}

// GetDesktopReadyData is not implemented on this platform
func GetDesktopReadyData() (*DesktopReadyData, error) {
	return nil, errors.New("bootlogon: not implemented on this platform")
}
```

### 2.5 Register the Check

The check needs to be registered in the agent's check catalog.

**Location**: Find where other system checks are registered (likely `pkg/collector/corechecks/system/` or a central registration file) and add:

```go
import "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/bootlogon"

// In registration:
RegisterCheck(bootlogon.CheckName, bootlogon.Factory())
```

---

## Part 3: IPC Protocol

The IPC protocol is simplified - the daemon only needs to ask the GUI about desktop ready status. All other timestamps are collected directly by the daemon via CGO.

### Request Format

```json
{"command": "get_desktop_ready"}
```

### Response Format (Success - Desktop Ready)

When `ready` is `true`, the agent logs the data and stops querying.

```json
{
  "success": true,
  "data": {
    "ready": true,
    "desktopReadyTime": 1707840010.345678,
    "username": "john",
    "uid": 501,
    "error": null
  },
  "error": null
}
```

### Response Format (Success - Desktop Not Ready)

When `ready` is `false`, the agent retries on the next check interval.

```json
{
  "success": true,
  "data": {
    "ready": false,
    "desktopReadyTime": null,
    "username": "john",
    "uid": 501,
    "error": null
  },
  "error": null
}
```

### Response Format (Error)

```json
{
  "success": false,
  "data": null,
  "error": "Desktop ready tracker not initialized"
}
```

---

## Part 4: Configuration

Add configuration options to `datadog.yaml`:

```yaml
## Boot and Logon Duration Check (macOS only) - PoC
## This check measures boot duration (power-on to login window)
## and logon duration (authentication to desktop ready).
## NOTE: This is a proof-of-concept that logs data only (no metrics/events sent).
#
# bootlogon_check:
#   ## @param enabled - boolean - optional - default: true
#   ## Enable the boot/logon duration check
#   #
#   # enabled: true
#
#   ## @param min_collection_interval - integer - optional - default: 5
#   ## Minimum interval between check runs in seconds.
#   ## The check only queries GUI until desktop is ready, then stops.
#   ## A shorter interval (5s) allows faster detection of desktop ready state.
#   #
#   # min_collection_interval: 5
```

---

## Part 5: FileVault Considerations

FileVault full-disk encryption significantly affects the boot timing sequence and requires special handling for accurate measurements.

### How FileVault Changes the Boot Sequence

**Without FileVault:**
```
Power On → Firmware → Kernel Boot (kern.boottime) → launchd → WindowServer starts (~1s)
                                                                     ↓
                                                    loginwindow starts (~2s after boot)
                                                                     ↓
                                                    "Login Window Application Started" logged
                                                                     ↓
                                                    Login UI visible → User enters credentials
                                                                     ↓
                                                    sessionDidLogin logged
```

**With FileVault:**
```
Power On → Firmware → Kernel Boot (kern.boottime) → launchd → WindowServer starts (~1s)
                                                                     ↓
                                                    FileVault unlock UI displayed by WindowServer
                                                                     ↓
                                                    *** User waits and enters FileVault password ***
                                                                     ↓
                                                    Disk unlocks, loginwindow process starts
                                                                     ↓
                                                    "Login Window Application Started" logged
                                                                     ↓
                                                    sessionDidLogin logged (~3-4s later, auto-login)
```

### The Problem

When FileVault is enabled, the `loginwindow` process **doesn't start until AFTER** the user has authenticated at the FileVault screen. This means:

1. All `loginwindow` log messages are delayed by however long the user waited at the FileVault screen
2. The time between `"Login Window Application Started"` and `sessionDidLogin` is always ~3-4 seconds (regardless of user wait time) because the FileVault credentials enable auto-login
3. Using `loginwindow` messages to measure boot duration incorrectly includes the user's wait time

**Example (FileVault enabled, user waited 15 seconds at unlock screen):**
- `kern.boottime`: 17:13:04
- WindowServer starts: 17:13:05 (~1s after boot) ← Login UI actually ready
- User enters FileVault password: 17:13:19 (waited ~15s)
- loginwindow starts: 17:13:20 ← This includes user wait time!
- sessionDidLogin: 17:13:24 (~4s after loginwindow)
- **Incorrect boot_duration**: 16 seconds (loginwindow - boot)
- **Correct boot_duration**: 1 second (WindowServer - boot)

### The Solution

The implementation uses WindowServer's startup message as the primary marker:

1. **Primary**: WindowServer `"Server is starting up"` message
   - Appears ~1 second after kernel boot
   - WindowServer displays the login/FileVault UI
   - Represents when the graphical system is ready for user input
   - Works correctly for both FileVault and non-FileVault systems

2. **Secondary**: WindowServer SkyLight subsystem messages
   - Alternative marker if primary not found

3. **Fallback**: `"Login Window Application Started"` message
   - Works well for non-FileVault systems
   - On FileVault systems, will include user wait time (less accurate)

4. **Last resort**: First `loginwindow` message

### Important Notes

- `kern.boottime` is recorded when the kernel starts, which is after any EFI-level operations
- WindowServer is responsible for displaying both the FileVault unlock screen and the regular login screen
- The `filevault` tag in logged data indicates whether FileVault is enabled, useful for analysis
- With FileVault, the user authenticates once (at FileVault screen) and is auto-logged in, so `logon_duration` measures the time from FileVault auth to desktop ready

---

## Part 6: Edge Cases

| Scenario | Behavior |
|----------|----------|
| **No console user** (SSH-only) | Check logs debug message, does nothing |
| **GUI not running** | Check can still get boot/login times via CGO; only desktop ready time unavailable |
| **Desktop not ready yet** | GUI returns `ready: false`; check retries on next interval |
| **Desktop ready** | GUI returns `ready: true`; check logs data once and stops querying |
| **System reboot** | Check detects boot time change, resets collection state, queries again |
| **Fast User Switching** | Daemon queries logs for current session; GUI tracks per-user ready state |
| **Auto-login enabled** | Logon duration will be very short (credentials to desktop) |
| **FileVault enabled** | Uses first loginwindow message for accurate timing; tagged for analysis |
| **FileVault pre-boot auth** | Pre-boot authentication time is NOT captured (occurs before kernel boot) |
| **System wake from sleep** | Unified Logging `com.apple.sessionDidLogin` only fires for true logins, not wake/unlock |
| **Headless installation** | Daemon still captures login times via CGO; desktop ready time unavailable |
| **Socket hijacking attempt** | Ownership validation rejects mismatched UIDs |
| **Agent restart during login** | Check re-queries CGO for timestamps and GUI for ready state |
| **CGO query failure** | Check logs warning, continues without those timestamps |

---

## Part 7: Testing

### Unit Tests

1. **Go side** (`bootlogon_test.go`):
   - Mock IPC responses with `ready: false` and `ready: true`
   - Test that check retries when `ready: false`
   - Test that check logs data and stops when `ready: true`
   - Test reboot detection (boot time change resets collection state)
   - Test `GetBootTime()` via sysctl
   - Test CGO functions (mock or integration test)
   - Test duration calculations with edge cases
   - Test sanity check boundaries

2. **Swift side**:
   - Test DesktopReadyTracker initialization
   - Test `ready` flag transitions (false → true when Finder launches)

### Integration Tests

1. Start GUI app, verify socket created
2. Send `get_desktop_ready` command before Finder launches, verify `ready: false`
3. Wait for Finder to launch, send command again, verify `ready: true`
4. Test CGO log queries return valid timestamps
5. Verify timestamps are reasonable (not in future, not too far in past)

### Manual Testing

```bash
# Check if GUI socket exists
ls -la /opt/datadog-agent/run/ipc/gui-*.sock

# Manually query the socket (requires nc/netcat)
# Check the "ready" field in the response
echo '{"command":"get_desktop_ready"}' | nc -U /opt/datadog-agent/run/ipc/gui-501.sock

# Check boot time via sysctl
sysctl kern.boottime

# Test unified log queries manually
log show --predicate 'processImagePath contains "loginwindow" and eventMessage contains "com.apple.sessionDidLogin"' --last 1d
log show --predicate 'processImagePath contains "loginwindow" and eventMessage contains "SessionAgentNotificationCenter"' --last 1d

# Verify agent logs - look for the "Boot/Logon Duration Data" block
tail -f /var/log/datadog/agent.log | grep bootlogon

# Expected log output when desktop is ready:
# bootlogon: ========== Boot/Logon Duration Data ==========
# bootlogon: username=john, uid=501, filevault=disabled
# bootlogon: boot_timestamp=1707839950.000
# bootlogon: login_window_timestamp=1707839990.123
# bootlogon: login_timestamp=1707840005.789
# bootlogon: desktop_ready_timestamp=1707840010.345
# bootlogon: boot_duration_seconds=40.12 (login_window - boot)
# bootlogon: logon_duration_seconds=4.56 (desktop_ready - login)
# bootlogon: ================================================
```

---

## Part 8: Implementation Checklist

### Phase 1: Swift GUI Changes (Simplified)

- [ ] Create `DesktopReadyTracker.swift` (Finder detection only)
- [ ] Add `DesktopReadyData` struct with `ready` flag and `desktopReadyTime`
- [ ] Implement Finder launch detection (notification + polling fallback)
- [ ] Add `DesktopReadyIPCResponse` struct to `WiFiIPCServer.swift`
- [ ] Update `WiFiIPCServer` init to accept `DesktopReadyTracker`
- [ ] Add `get_desktop_ready` command handler
- [ ] Update `gui.swift` to initialize `DesktopReadyTracker`
- [ ] Update `gui.swift` to pass `DesktopReadyTracker` to `WiFiIPCServer`
- [ ] Test IPC manually - verify `ready` flag transitions

### Phase 2: Go Agent Check with CGO

- [ ] Create `pkg/collector/corechecks/system/bootlogon/` directory
- [ ] Create `bootlogon.go` with check implementation
  - [ ] Implement `ready` flag checking logic
  - [ ] Implement one-time collection (stop querying after `ready: true`)
  - [ ] Implement reboot detection (reset state when boot time changes)
  - [ ] Implement logging of boot/logon data (PoC - no metrics/events)
- [ ] Create `bootlogon_darwin.go` with CGO implementation
  - [ ] Implement `GetBootTime()` via sysctl
  - [ ] Implement `GetLoginWindowTime()` via CGO to OSLogStore
  - [ ] Implement `GetLoginTime()` via CGO to OSLogStore
  - [ ] Implement `IsFileVaultEnabled()` via CGO
  - [ ] Implement `GetDesktopReadyData()` via IPC
- [ ] Create `bootlogon_darwin.m` with Objective-C OSLogStore queries
  - [ ] Implement `queryLoginTimestamp()` for both login window and login time
  - [ ] Implement `checkFileVaultEnabled()`
- [ ] Create `bootlogon_noop.go` for other platforms
- [ ] Register check in catalog
- [ ] Add unit tests
- [ ] Test end-to-end

### Phase 3: Documentation (PoC)

- [ ] Update agent configuration documentation
- [ ] Document expected log output format

### Future Work (Post-PoC)

- [ ] Convert logging to Datadog Events
- [ ] Add metrics documentation
- [ ] Add to release notes

---

## References

- Existing WiFi IPC implementation: `pkg/collector/corechecks/net/wlan/wlan_darwin.go`
- GUI Swift sources: `comp/core/gui/guiimpl/systray/Sources/`
- NSWorkspace notifications: [Apple Documentation](https://developer.apple.com/documentation/appkit/nsworkspace)
- sysctl kern.boottime: Standard BSD/macOS kernel interface

### Unified Logging References

- OSLogStore API: [Apple Documentation](https://developer.apple.com/documentation/oslog/oslogstore)
- OSLogStore.local(): [Apple Documentation](https://developer.apple.com/documentation/oslog/oslogstore/local())
- Analysis of Apple Unified Logs for Login Detection: [mac4n6.com - Login Week](https://www.mac4n6.com/blog/2020/4/26/analysis-of-apple-unified-logs-quarantine-edition-entry-4-its-login-week)
- Log predicates reference: Run `log help predicates` in Terminal


### Key Unified Log Predicates for Login Detection

```bash
# True login events (login from login window, not unlock)
log show --predicate 'processImagePath contains "loginwindow" and eventMessage contains "com.apple.sessionDidLogin"'

# All session events (login, logout, lock, unlock, restart, shutdown)
log show --predicate 'eventMessage contains "SessionAgentNotificationCenter"'

# Authentication method (password, Touch ID, Apple Watch)
log show --predicate 'eventMessage contains "LWScreenLockAuthentication" and (eventMessage contains "| Verifying" or eventMessage contains "| Using")'

# Screen lock/unlock status
log show --predicate 'eventMessage contains "com.apple.sessionagent.screenIs"'
```
