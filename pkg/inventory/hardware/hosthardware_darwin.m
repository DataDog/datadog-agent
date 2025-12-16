#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>
#import "hosthardware_darwin.h"
#import <stdlib.h>
#import <string.h>

static NSDictionary *getServiceProperties(CFMutableDictionaryRef matching) {
    io_service_t service = IOServiceGetMatchingService(kIOMainPortDefault, matching);
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
        return strdup([value UTF8String]);
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
            info.found = true;
            info.modelIdentifier = copyStringProperty(platform, @"model");
            info.serialNumber = copyStringProperty(platform, @"IOPlatformSerialNumber");

            char *modelNum = copyStringProperty(platform, @"model-number");
            char *region = copyStringProperty(platform, @"region-info");
            if (modelNum && region) {
                asprintf(&info.modelNumber, "%s%s", modelNum, region);
                free(modelNum);
                free(region);
            } else {
                info.modelNumber = modelNum;
                free(region);
            }
        }

        NSDictionary *product = getServiceProperties(IOServiceNameMatching("product"));
        if (product) {
            info.productName = copyStringProperty(product, @"product-name");
        }

        return info;
    }
}

