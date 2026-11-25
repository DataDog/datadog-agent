// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>
#import <IOKit/kext/KextManager.h>
#import <SystemConfiguration/SystemConfiguration.h>
#import "nicinfo_darwin.h"
#import <stdlib.h>
#import <string.h>

// kIOMainPortDefault was introduced in macOS 12.0, use kIOMasterPortDefault for older versions
#if __MAC_OS_X_VERSION_MIN_REQUIRED >= 120000
#define IOKIT_MAIN_PORT kIOMainPortDefault
#else
#define IOKIT_MAIN_PORT kIOMasterPortDefault
#endif

// copyProperty returns the property of an entry, or of its parents with
// kIORegistryIterateParents, or nil.
static id copyProperty(io_registry_entry_t entry, CFStringRef key, IOOptionBits options) {
    CFTypeRef value = IORegistryEntrySearchCFProperty(entry, kIOServicePlane, key, kCFAllocatorDefault, options);
    return CFBridgingRelease(value);
}

// stringProperty returns the property like copyProperty, or nil when it is not a string.
static NSString *stringProperty(io_registry_entry_t entry, CFStringRef key, IOOptionBits options) {
    id value = copyProperty(entry, key, options);
    return [value isKindOfClass:[NSString class]] ? value : nil;
}

// copyString returns a copy of value to free with free, or NULL when value is nil.
static char *copyString(NSString *value) {
    const char *utf8 = [value UTF8String];
    return utf8 ? strdup(utf8) : NULL;
}

// copyDriver returns the entry of the driver of a controller. The controllers of DriverKit drivers,
// such as the Wi-Fi one, are kernel objects whose parent with the same MAC address is the driver.
static io_registry_entry_t copyDriver(io_registry_entry_t controller, id mac) {
    io_registry_entry_t parent;
    if (mac && !copyProperty(controller, CFSTR("IOUserClass"), 0) &&
        IORegistryEntryGetParentEntry(controller, kIOServicePlane, &parent) == KERN_SUCCESS) {
        if ([mac isEqual:copyProperty(parent, CFSTR("IOMACAddress"), 0)] && copyProperty(parent, CFSTR("IOUserClass"), 0)) {
            return parent;
        }
        IOObjectRelease(parent);
    }
    IOObjectRetain(controller);
    return controller;
}

// addController adds the properties of the controller of an interface, which is its parent in the
// IOKit registry.
static void addController(io_registry_entry_t controller, NSMutableDictionary *entry) {
    id mac = copyProperty(controller, CFSTR("IOMACAddress"), 0);
    if (![mac isKindOfClass:[NSData class]]) {
        mac = nil;
    }
    if (mac) {
        entry[@"mac"] = mac;
    }

    // USB adapters leave IOVendor and IOModel unset, and their USB device has the names
    IOOptionBits parents = kIORegistryIterateRecursively | kIORegistryIterateParents;
    NSString *vendor = stringProperty(controller, CFSTR("IOVendor"), 0) ?: stringProperty(controller, CFSTR("USB Vendor Name"), parents);
    NSString *model = stringProperty(controller, CFSTR("IOModel"), 0) ?: stringProperty(controller, CFSTR("USB Product Name"), parents);
    if (vendor) {
        entry[@"vendor"] = vendor;
    }
    if (model) {
        entry[@"model"] = model;
    }

    io_registry_entry_t driverEntry = copyDriver(controller, mac);
    NSString *driverClass = stringProperty(driverEntry, CFSTR("IOUserClass"), 0);
    io_name_t className;
    if (!driverClass && IOObjectGetClass(driverEntry, className) == KERN_SUCCESS) {
        driverClass = @(className);
    }
    if (driverClass) {
        entry[@"driverClass"] = driverClass;
    }
    NSString *driver = stringProperty(driverEntry, CFSTR("CFBundleIdentifier"), 0);
    if (driver) {
        entry[@"driver"] = driver;
    }
    IOObjectRelease(driverEntry);
}

// addRegistryInterfaces adds the IONetworkInterface entries of the IOKit registry and their
// controllers to byName. It returns NO when it cannot list them.
static BOOL addRegistryInterfaces(NSMutableDictionary<NSString *, NSMutableDictionary *> *byName) {
    io_iterator_t iterator;
    if (IOServiceGetMatchingServices(IOKIT_MAIN_PORT, IOServiceMatching("IONetworkInterface"), &iterator) != KERN_SUCCESS) {
        return NO;
    }
    io_service_t service;
    while ((service = IOIteratorNext(iterator))) {
        NSString *name = stringProperty(service, CFSTR("BSD Name"), 0);
        if (name) {
            NSMutableDictionary *entry = byName[name] ?: [NSMutableDictionary dictionary];
            byName[name] = entry;
            entry[@"inRegistry"] = @YES;
            id builtin = copyProperty(service, CFSTR("IOBuiltin"), 0);
            if ([builtin isKindOfClass:[NSNumber class]]) {
                entry[@"builtin"] = builtin;
            }
            io_registry_entry_t controller;
            if (IORegistryEntryGetParentEntry(service, kIOServicePlane, &controller) == KERN_SUCCESS) {
                addController(controller, entry);
                IOObjectRelease(controller);
            }
        }
        IOObjectRelease(service);
    }
    IOObjectRelease(iterator);
    return YES;
}

// addDisplayNames adds the SystemConfiguration display names of the interfaces to byName. It
// returns NO when it cannot list them.
static BOOL addDisplayNames(NSMutableDictionary<NSString *, NSMutableDictionary *> *byName) {
    CFArrayRef interfaces = SCNetworkInterfaceCopyAll();
    if (!interfaces) {
        return NO;
    }
    for (CFIndex i = 0; i < CFArrayGetCount(interfaces); i++) {
        SCNetworkInterfaceRef interface = (SCNetworkInterfaceRef)CFArrayGetValueAtIndex(interfaces, i);
        NSString *name = (__bridge NSString *)SCNetworkInterfaceGetBSDName(interface);
        NSString *displayName = (__bridge NSString *)SCNetworkInterfaceGetLocalizedDisplayName(interface);
        if (name && displayName) {
            NSMutableDictionary *entry = byName[name] ?: [NSMutableDictionary dictionary];
            byName[name] = entry;
            entry[@"description"] = displayName;
        }
    }
    CFRelease(interfaces);
    return YES;
}

// addDriverVersions adds the version of the loaded kernel extension of each driver to byName.
static void addDriverVersions(NSMutableDictionary<NSString *, NSMutableDictionary *> *byName) {
    NSMutableSet<NSString *> *drivers = [NSMutableSet set];
    for (NSDictionary *entry in byName.allValues) {
        if (entry[@"driver"]) {
            [drivers addObject:entry[@"driver"]];
        }
    }
    if (drivers.count == 0) {
        return;
    }
    // DriverKit drivers are not kernel extensions, so they get no version
    NSDictionary *kexts = CFBridgingRelease(KextManagerCopyLoadedKextInfo((__bridge CFArrayRef)drivers.allObjects, (__bridge CFArrayRef)@[@"CFBundleVersion"]));
    for (NSMutableDictionary *entry in byName.allValues) {
        NSString *version = kexts[entry[@"driver"] ?: @""][@"CFBundleVersion"];
        if ([version isKindOfClass:[NSString class]]) {
            entry[@"driverVersion"] = version;
        }
    }
}

// copyNICInfo merges what IOKit and SystemConfiguration report about each interface.
NICInfo *copyNICInfo(int *count, int *failed) {
    @autoreleasepool {
        *count = 0;
        *failed = 0;
        NSMutableDictionary<NSString *, NSMutableDictionary *> *byName = [NSMutableDictionary dictionary];
        if (!addRegistryInterfaces(byName) || !addDisplayNames(byName)) {
            *failed = 1;
            return NULL;
        }
        addDriverVersions(byName);

        if (byName.count == 0) {
            return NULL;
        }
        NICInfo *infos = calloc(byName.count, sizeof(NICInfo));
        if (!infos) {
            *failed = 1;
            return NULL;
        }
        for (NSString *name in byName) {
            NSDictionary *entry = byName[name];
            NICInfo *info = &infos[(*count)++];
            info->name = copyString(name);
            info->description = copyString(entry[@"description"]);
            info->vendor = copyString(entry[@"vendor"]);
            info->model = copyString(entry[@"model"]);
            info->driverClass = copyString(entry[@"driverClass"]);
            info->driver = copyString(entry[@"driver"]);
            info->driverVersion = copyString(entry[@"driverVersion"]);
            NSData *mac = entry[@"mac"];
            if (mac.length > 0 && mac.length <= sizeof(info->mac)) {
                memcpy(info->mac, mac.bytes, mac.length);
                info->macLength = (int)mac.length;
            }
            info->inRegistry = entry[@"inRegistry"] != nil;
            info->builtin = [entry[@"builtin"] boolValue];
        }
        return infos;
    }
}

// freeNICInfo frees the interfaces of copyNICInfo and their strings.
void freeNICInfo(NICInfo *infos, int count) {
    for (int i = 0; i < count; i++) {
        free(infos[i].name);
        free(infos[i].description);
        free(infos[i].vendor);
        free(infos[i].model);
        free(infos[i].driverClass);
        free(infos[i].driver);
        free(infos[i].driverVersion);
    }
    free(infos);
}
