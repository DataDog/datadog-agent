# macOS Boot/Logon Duration Collection - Solutions Analysis

This document tracks the various approaches explored for collecting boot and logon duration data on macOS, including their limitations and status.

## Overview

Collecting accurate boot/logon timing data on macOS is challenging due to:
1. Timestamps are scattered across different sources (kernel, unified logs, GUI events)
2. Access to unified logs requires elevated privileges or entitlements
3. FileVault changes the boot sequence significantly
4. Agent installation mode affects available permissions

### Agent Installation Modes

| Install Type | Launch Location | Runs As | Log Access |
|--------------|-----------------|---------|------------|
| **System-wide** (LaunchDaemon) | `/Library/LaunchDaemons/` | root | May work with entitlements |
| **Per-user** (LaunchAgent) | `~/Library/LaunchAgents/` | User's UID | Depends on user account type |

**Per-user installs** are common for:
- Non-admin users who can't install system-wide
- Testing/development setups
- Enterprise deployments with user-level management

With per-user installs, the agent inherits the user's permissions, so:
- **Admin user**: OSLogStore *may* work (but entitlements still required)
- **Standard user**: OSLogStore will fail (no access to local log store)

---

## Approach 1: GUI App Notifications (Initial Attempt)

**Status**: ❌ Not Viable (timing issue)

### Description
The initial approach attempted to capture login timestamps directly in the GUI app (Swift) by registering for system notifications related to login events.

### Implementation Concept
- The GUI app (LaunchAgent) would register for `NSWorkspace` or `DistributedNotificationCenter` notifications
- Notifications like session login, screen unlock, or user switch events would be captured
- Timestamps would be recorded when notifications were received

### Example Notifications Considered
```swift
// NSWorkspace notifications
NSWorkspace.sessionDidBecomeActiveNotification
NSWorkspace.sessionDidResignActiveNotification

// Distributed notifications  
"com.apple.screenIsLocked"
"com.apple.screenIsUnlocked"
"com.apple.sessionDidLogin"
```

### Why It Failed

**The fundamental timing problem**: The GUI app is a LaunchAgent that starts **during** the user login process. By the time the app launches and registers for notifications, the login-related events have **already occurred**.

```
Boot Sequence:
  Kernel boot
  → Login window appears
  → User enters credentials
  → Session starts, LaunchAgents begin launching
  → GUI app starts ← TOO LATE, login events already fired
  → GUI app registers for notifications
  → (notifications about login never received)
```

The GUI app cannot capture events that happened before it started running. This is a fundamental limitation of the LaunchAgent architecture for capturing login timing.

### What Still Works
The GUI app **can** successfully detect:
- `desktop_ready_timestamp`: When Finder launches (happens after GUI app starts)
- User session information (username, UID)

These are captured via `NSWorkspace.didLaunchApplicationNotification` and polling for Finder.

### Conclusion
This approach was abandoned because it cannot capture the timestamps needed for boot/login duration calculations. The OSLogStore approach (Approach 2) was explored as an alternative since unified logs retain historical events that can be queried after the fact.

---

## Approach 2: OSLogStore API via CGO (Current Implementation)

**Status**: ⚠️ Partially Working (requires entitlements)

### Description
Uses Objective-C/CGO to query the macOS Unified Logging system via `OSLogStore` API to retrieve login timestamps.

### Implementation
- File: `pkg/collector/corechecks/system/bootlogon/bootlogon_darwin.m`
- Queries `OSLogStore.localStoreAndReturnError:` to access local logs
- Searches for specific log messages (WindowServer, loginwindow)

### Timestamps Collected
| Timestamp | Source | Method |
|-----------|--------|--------|
| `boot_timestamp` | `kern.boottime` via sysctl | ✅ Works (no special permissions) |
| `login_window_timestamp` | Unified Logs (WindowServer/loginwindow) | ⚠️ Requires admin account |
| `login_timestamp` | Unified Logs (`com.apple.sessionDidLogin`) | ⚠️ Requires admin account |
| `desktop_ready_timestamp` | GUI IPC (Finder detection) | ✅ Works |

### Log Messages Used

#### `login_window_timestamp` - Evolution of Approach

**Original approach (v1):**
```
Process: loginwindow
Message: "Login Window Application Started"
```

This worked correctly **without FileVault** (~2 seconds after boot), but with FileVault enabled it showed ~28 seconds, which was incorrect.

**FileVault Issue Explained:**

With FileVault enabled, the boot sequence is different:

```
Without FileVault:
  Kernel boot → WindowServer starts → loginwindow starts → "Login Window Application Started" (~2s)
                                                         → User sees login UI
                                                         → User enters password
                                                         → sessionDidLogin

With FileVault:
  Kernel boot → WindowServer starts (~1s) → FileVault unlock UI displayed
                                          → *** User waits and enters FileVault password ***
                                          → Disk unlocks
                                          → loginwindow starts → "Login Window Application Started"
                                          → sessionDidLogin (~3-4s later, auto-login)
```

The key problem: **loginwindow doesn't start until AFTER FileVault authentication completes**. The FileVault unlock UI is displayed by WindowServer, not loginwindow. So any loginwindow message includes however long the user waited at the FileVault screen.

This explained why the time between "Login Window Application Started" and "sessionDidLogin" was always ~3-4 seconds regardless of how long the user waited - the FileVault credentials enable auto-login.

**Fixed approach (v2):**
```
Process: WindowServer
Message: "Server is starting up"
```

WindowServer starts ~1 second after kernel boot and is responsible for displaying both the FileVault unlock screen and the regular login window. This timestamp correctly represents when the graphical system is ready for user input, regardless of FileVault status.

**Fallback stages (in order):**
1. WindowServer `"Server is starting up"` (primary)
2. WindowServer SkyLight subsystem messages
3. loginwindow `"Login Window Application Started"` (fallback for non-FileVault)
4. First loginwindow message (last resort)

#### `login_timestamp`
```
Process: loginwindow
Message contains: "com.apple.sessionDidLogin"
```

This captures when the user successfully authenticated and the session started.

### Limitations

#### Entitlement Requirement
The OSLogStore API requires the `com.apple.private.logging.admin` entitlement (or similar) to access the local log store:

```
Error Domain=OSLogErrorDomain Code=9 "Client lacks entitlement to perform operation"
```

Based on testing:
- ✅ **Works**: Per-user install (LaunchAgent) + **admin account**
- ❌ **Fails**: Per-user install (LaunchAgent) + **standard account**

The key factor is the **user's account type** (admin vs standard), not the installation type. Standard users don't have permission to access the local log store via OSLogStore API, even when running processes they own.

#### Tested Scenarios
| Scenario | Result |
|----------|--------|
| Admin account on VM | ✅ Works |
| Standard account on VM | ❌ Fails (entitlement error) |
| Admin account on physical Mac | ❓ Untested |
| Standard account on physical Mac | ❌ Fails (entitlement error) |

### Code Reference

```objc
// bootlogon_darwin.m
OSLogStore *logStore = [OSLogStore localStoreAndReturnError:&error];
// Fails with: "Client lacks entitlement to perform operation"
```

---

## Approach 3: `log show` Command (Shell)

**Status**: ❌ Not Viable (same permission issues)

### Description
Shell out to the macOS `log` command to query unified logs and parse the output.

### Example Command
```bash
log show --predicate 'process == "loginwindow"' --last boot --style compact
```

### Limitations

The `log` command has the same permission restrictions as the OSLogStore API:

```
❯ log show
log: Could not open local log store: Operation not permitted
```

This occurs because:
1. The `log` command internally uses the same OSLogStore API
2. Standard users don't have access to the local log store
3. Even `sudo log show` may fail without proper entitlements on the calling binary

### When It Works
- Works in Terminal.app because Terminal has TCC (Transparency, Consent, Control) permissions
- May work if the agent binary has Full Disk Access in System Preferences
- Works for admin users in interactive sessions

### When It Fails
- LaunchDaemon context without proper entitlements
- Standard user accounts
- Automated/non-interactive contexts

---

## Current Workaround

For systems where the OSLogStore API fails due to entitlements, the check gracefully degrades:

1. `boot_timestamp` is always available (via sysctl)
2. `login_window_timestamp` and `login_timestamp` will be missing
3. `desktop_ready_timestamp` is available if the GUI app is running
4. The check logs a warning but continues to function

### Partial Data Collected
```
bootlogon: boot_timestamp=1771541003.000 (2026-02-19 17:43:23)
bootlogon: login_window_timestamp=(unavailable - entitlement required)
bootlogon: login_timestamp=(unavailable - entitlement required)  
bootlogon: desktop_ready_timestamp=1771541038.268 (2026-02-19 17:43:58)
```

---

## Recommendations

### Short Term
1. Document the entitlement requirement clearly
2. Ensure graceful degradation when unified log access fails
3. Investigate if Full Disk Access TCC permission helps

### Medium Term
1. Investigate LaunchAgent approach for log collection
2. Research if any public entitlements can provide log access
3. Consider requesting `com.apple.private.logging.admin` from Apple

### Long Term
1. Evaluate System Extension / Endpoint Security approach
2. Consider if the data is valuable enough to justify the complexity
3. Monitor Apple's changes to unified logging permissions

---

## Testing Matrix

| macOS Version | Install Type | Account Type | FileVault | OSLogStore | `log show` | Boot Data |
|---------------|--------------|--------------|-----------|------------|------------|-----------|
| 14.x (VM) | Per-user | Admin | Enabled | ✅ | ✅ | Full |
| 14.x (VM) | Per-user | Admin | Disabled | ✅ | ✅ | Full |
| 14.x (Physical) | Per-user | Standard | Enabled | ❌ | ❌ | Partial |
| 14.x (Physical) | Per-user | Standard | Disabled | ❓ | ❓ | TBD |
| Any | System-wide | Admin | Any | ❓ | ❓ | TBD |
| Any | System-wide | Standard | Any | ❓ | ❓ | TBD |

**Key Finding**: The user's account type (admin vs standard) determines access to the local log store. Standard users cannot access OSLogStore regardless of installation type.

---

## References

- [Apple Unified Logging Documentation](https://developer.apple.com/documentation/os/logging)
- [OSLogStore API](https://developer.apple.com/documentation/oslog/oslogstore)
- [Entitlements for System Services](https://developer.apple.com/documentation/bundleresources/entitlements)
- [The Eclectic Light Company - Unified Log Analysis](https://eclecticlight.co/2018/03/21/macos-unified-log-3-finding-your-way/)
