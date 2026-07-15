#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>
#import <IOKit/IOKitKeys.h>
#import <IOKit/ps/IOPowerSources.h>
#import <IOKit/ps/IOPSKeys.h>
#import "battery_darwin.h"

// kIOMainPortDefault was introduced in macOS 12.0, use kIOMasterPortDefault for older versions
#if __MAC_OS_X_VERSION_MIN_REQUIRED >= 120000
#define IOKIT_MAIN_PORT kIOMainPortDefault
#else
#define IOKIT_MAIN_PORT kIOMasterPortDefault
#endif

// hasInternalBattery reports whether macOS sees a real internal battery via the
// PowerSources API. This avoids false positives on Mac minis, where an
// AppleSmartBattery IOKit stub may be present even though no battery exists.
bool hasInternalBattery(void) {
    CFTypeRef blob = IOPSCopyPowerSourcesInfo();
    if (blob == NULL) {
        return false;
    }
    CFArrayRef list = IOPSCopyPowerSourcesList(blob);
    if (list == NULL) {
        CFRelease(blob);
        return false;
    }
    bool found = false;
    CFIndex count = CFArrayGetCount(list);
    for (CFIndex i = 0; i < count; i++) {
        CFTypeRef ps = CFArrayGetValueAtIndex(list, i);
        CFDictionaryRef desc = IOPSGetPowerSourceDescription(blob, ps);
        if (desc == NULL) {
            continue;
        }
        CFStringRef type = CFDictionaryGetValue(desc, CFSTR(kIOPSTypeKey));
        if (type != NULL &&
            CFStringCompare(type, CFSTR(kIOPSInternalBatteryType), 0) == kCFCompareEqualTo) {
            found = true;
            break;
        }
    }
    CFRelease(list);
    CFRelease(blob);
    return found;
}

// batteryServiceConditionReported reports whether macOS considers the internal
// battery to be in a degraded/"Service Recommended" state — the macOS equivalent
// of the Windows power_state:battery_critical signal.
//
// NOTE: this is a battery *health* condition (persistent hardware degradation),
// which differs from the Windows BATTERY_CRITICAL flag (transient critically-low
// *charge*). Both intentionally share the power_state:battery_critical tag.
//
// We deliberately avoid the documented kIOPSBatteryHealthKey ("BatteryHealth"):
// it is unreliable on Apple Silicon and reports "Check Battery" even on healthy
// batteries. Instead we read, from the same PowerSources description the rest of
// the check already uses:
//   1. kIOPSBatteryHealthConditionKey — documented, returns "Check Battery" /
//      "Permanent Battery Failure" (empty when healthy); and
//   2. "Battery Service State" — the private/undocumented integer service-flags
//      key that Apple's own SPPowerReporter uses to drive the Settings UI. It is
//      absent from the description when the battery is healthy. Read defensively.
//
// NOTE: on the one real failing battery we validated against (stuck at 1%,
// "Service Recommended" in Settings), BOTH of these PowerSources signals were
// empty/absent — only PermanentFailureStatus (checked in getBatteryInfo) was
// set. They are kept here because they catch service conditions that don't latch
// a permanent-failure bit; the OR across all three is intentional.
static bool batteryServiceConditionReported(void) {
    bool critical = false;

    CFTypeRef blob = IOPSCopyPowerSourcesInfo();
    if (blob == NULL) {
        return false;
    }
    CFArrayRef list = IOPSCopyPowerSourcesList(blob);
    if (list == NULL) {
        CFRelease(blob);
        return false;
    }

    CFIndex count = CFArrayGetCount(list);
    for (CFIndex i = 0; i < count; i++) {
        CFTypeRef ps = CFArrayGetValueAtIndex(list, i);
        CFDictionaryRef desc = IOPSGetPowerSourceDescription(blob, ps);
        if (desc == NULL) {
            continue;
        }

        CFStringRef type = CFDictionaryGetValue(desc, CFSTR(kIOPSTypeKey));
        if (type == NULL ||
            CFStringCompare(type, CFSTR(kIOPSInternalBatteryType), 0) != kCFCompareEqualTo) {
            continue;
        }

        // Documented public condition key.
        CFStringRef condition = CFDictionaryGetValue(desc, CFSTR(kIOPSBatteryHealthConditionKey));
        if (condition != NULL &&
            (CFStringCompare(condition, CFSTR(kIOPSCheckBatteryValue), 0) == kCFCompareEqualTo ||
             CFStringCompare(condition, CFSTR(kIOPSPermanentFailureValue), 0) == kCFCompareEqualTo)) {
            critical = true;
            break;
        }

        // Private/undocumented service-flags key used by SPPowerReporter. Absent
        // when healthy; any non-zero value indicates a service condition.
        CFNumberRef serviceState = CFDictionaryGetValue(desc, CFSTR("Battery Service State"));
        if (serviceState != NULL) {
            int serviceValue = 0;
            if (CFNumberGetValue(serviceState, kCFNumberIntType, &serviceValue) && serviceValue != 0) {
                critical = true;
                break;
            }
        }
    }

    CFRelease(list);
    CFRelease(blob);
    return critical;
}

static NSDictionary *getSmartBatteryProperties(void) {
    CFMutableDictionaryRef matching = IOServiceMatching("AppleSmartBattery");
    if (matching == NULL) {
        return nil;
    }

    io_service_t batteryEntry = IOServiceGetMatchingService(IOKIT_MAIN_PORT, matching);
    if (batteryEntry == 0) {
        return nil;
    }

    CFMutableDictionaryRef props = NULL;
    if (IORegistryEntryCreateCFProperties(batteryEntry, &props, kCFAllocatorDefault, 0) != KERN_SUCCESS) {
        IOObjectRelease(batteryEntry);
        return nil;
    }

    IOObjectRelease(batteryEntry);
    return (__bridge_transfer NSDictionary *)props;
}

BatteryInfo getBatteryInfo(void) {
    @autoreleasepool {
        BatteryInfo info = {0};

        NSDictionary *props = getSmartBatteryProperties();
        if (!props) {
            return info;
        }

        if (props[@"DesignCapacity"] != nil) {
            info.designCapacity = (OptionalInt){true, [props[@"DesignCapacity"] intValue]};
        }
        if (props[@"AppleRawMaxCapacity"] != nil) {
            info.appleRawMaxCapacity = (OptionalInt){true, [props[@"AppleRawMaxCapacity"] intValue]};
        }
        if (props[@"CycleCount"] != nil) {
            info.cycleCount = (OptionalInt){true, [props[@"CycleCount"] intValue]};
        }
        if (props[@"CurrentCapacity"] != nil) {
            info.currentCapacity = (OptionalInt){true, [props[@"CurrentCapacity"] intValue]};
        }
        if (props[@"Voltage"] != nil) {
            info.voltage = (OptionalInt){true, [props[@"Voltage"] intValue]};
        }
        if (props[@"InstantAmperage"] != nil) {
            info.instantAmperage = (OptionalInt){true, [props[@"InstantAmperage"] intValue]};
        }
        if (props[@"IsCharging"] != nil) {
            info.isCharging = (OptionalBool){true, [props[@"IsCharging"] boolValue]};
        }
        if (props[@"ExternalConnected"] != nil) {
            info.externalConnected = (OptionalBool){true, [props[@"ExternalConnected"] boolValue]};
        }

        // Degraded/"Service Recommended" detection. We have successfully read the
        // battery, so we can definitively report critical-or-not. PermanentFailureStatus
        // is a deterministic permanent-hardware-failure flag from this same dict
        // (0 when healthy); it was the signal that actually fired (0x2000) on the
        // real failing battery this feature was validated against, while the
        // PowerSources health signals were empty. We OR the two so we also catch
        // service conditions that don't latch a permanent-failure bit.
        bool critical = batteryServiceConditionReported();
        if (props[@"PermanentFailureStatus"] != nil && [props[@"PermanentFailureStatus"] intValue] != 0) {
            critical = true;
        }
        info.isCritical = (OptionalBool){true, critical};

        return info;
    }
}
