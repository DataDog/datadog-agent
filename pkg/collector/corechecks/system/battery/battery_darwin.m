#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>
#import <IOKit/IOKitKeys.h>
#import "battery_darwin.h"

static NSDictionary *getSmartBatteryProperties(void) {
    CFMutableDictionaryRef matching = IOServiceMatching("AppleSmartBattery");
    if (matching == NULL) {
        return nil;
    }

    io_service_t batteryEntry = IOServiceGetMatchingService(kIOMainPortDefault, matching);
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
        info.cycleCount = [props[@"CycleCount"] intValue];
        info.designCapacity = [props[@"DesignCapacity"] intValue];
        info.appleRawMaxCapacity = [props[@"AppleRawMaxCapacity"] intValue];
        info.currentCapacity = [props[@"CurrentCapacity"] intValue];
        info.voltage = [props[@"Voltage"] intValue];
        info.instantAmperage = [props[@"InstantAmperage"] intValue];
        info.isCharging = [props[@"IsCharging"] boolValue];
        info.externalConnected = [props[@"ExternalConnected"] boolValue];

        return info;
    }
}
