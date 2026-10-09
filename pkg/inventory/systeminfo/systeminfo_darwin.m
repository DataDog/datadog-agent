#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>
#import <SystemConfiguration/SystemConfiguration.h>
#import "systeminfo_darwin.h"
#import <stdlib.h>
#import <string.h>

// kIOMainPortDefault was introduced in macOS 12.0, use kIOMasterPortDefault for older versions
#if __MAC_OS_X_VERSION_MIN_REQUIRED >= 120000
#define IOKIT_MAIN_PORT kIOMainPortDefault
#else
#define IOKIT_MAIN_PORT kIOMasterPortDefault
#endif

static NSDictionary *getServiceProperties(CFMutableDictionaryRef matching) {
    io_service_t service = IOServiceGetMatchingService(IOKIT_MAIN_PORT, matching);
    if (!service) return nil;

    CFMutableDictionaryRef props = NULL;
    if (IORegistryEntryCreateCFProperties(service, &props, kCFAllocatorDefault, 0) != KERN_SUCCESS) {
        IOObjectRelease(service);
        return nil;
    }

    IOObjectRelease(service);
    return (__bridge_transfer NSDictionary *)props;
}

static char *copyStringProperty(NSDictionary *props, NSString *key) {
    id value = props[key];
    if (!value) return NULL;
    
    if ([value isKindOfClass:[NSString class]]) {
        const char *utf8 = [value UTF8String];
        return utf8 ? strdup(utf8) : NULL;
    } else if ([value isKindOfClass:[NSData class]]) {
        const char *bytes = (const char *)[value bytes];
        return strndup(bytes, strnlen(bytes, [value length]));
    }
    return NULL;
}

DeviceInfo getDeviceInfo(void) {
    @autoreleasepool {
        DeviceInfo info = {0};

        NSDictionary *platform = getServiceProperties(IOServiceMatching("IOPlatformExpertDevice"));
        if (platform) {
            info.modelIdentifier = copyStringProperty(platform, @"model");
            info.serialNumber = copyStringProperty(platform, @"IOPlatformSerialNumber");

            char *modelNum = copyStringProperty(platform, @"model-number");
            char *region = copyStringProperty(platform, @"region-info");
            if (modelNum && region) {
                // info.modelNumber should be NULL in call to asprintf to avoid memory leak
                asprintf(&info.modelNumber, "%s%s", modelNum, region);
                free(modelNum);
            } else if (modelNum) {
                info.modelNumber = modelNum;
            }
            // Always free region (should be safe if NULL)
            free(region);
        }

        NSDictionary *product = getServiceProperties(IOServiceNameMatching("product"));
        if (product) {
            info.productName = copyStringProperty(product, @"product-name");
        }
        if (!product || !info.productName) {
            // Fallback to modelIdentifier if available, otherwise use empty string
            if (info.modelIdentifier) {
                info.productName = strdup(info.modelIdentifier);
            } else {
                info.productName = strdup("");
            }
        }

        // Read the configured Computer Name, not the dynamic-store name that
        // can reflect network name-conflict handling. Never write preferences.
        SCPreferencesRef preferences = SCPreferencesCreate(NULL, CFSTR("Datadog Agent"), NULL);
        if (preferences) {
            CFPropertyListRef system = SCPreferencesPathGetValue(preferences, CFSTR("/System/System"));
            if (system && CFGetTypeID(system) == CFDictionaryGetTypeID()) {
                info.computerName = copyStringProperty((__bridge NSDictionary *)system,
                                                       (__bridge NSString *)kSCPropSystemComputerName);
            }
            CFRelease(preferences);
        }
        if (!info.computerName) {
            info.computerName = strdup("");
        }

        return info;
    }
}

