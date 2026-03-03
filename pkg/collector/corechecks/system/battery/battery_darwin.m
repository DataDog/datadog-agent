#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>
#import <IOKit/IOKitKeys.h>
#import "battery_darwin.h"

// kIOMainPortDefault was introduced in macOS 12.0, use kIOMasterPortDefault for older versions
#if __MAC_OS_X_VERSION_MIN_REQUIRED >= 120000
#define IOKIT_MAIN_PORT kIOMainPortDefault
#else
#define IOKIT_MAIN_PORT kIOMasterPortDefault
#endif

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

        info.found = true;

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

        return info;
    }
}
