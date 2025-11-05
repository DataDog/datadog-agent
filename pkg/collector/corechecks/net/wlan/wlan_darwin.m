#import <Foundation/Foundation.h>
#import <CoreLocation/CoreLocation.h>
#import <CoreWLAN/CoreWLAN.h>
#import "wlan_darwin.h"

WiFiInfo GetWiFiInformation() {
    WiFiInfo info = {0};

    @autoreleasepool {
        CWInterface *wifiInterface = [[CWWiFiClient sharedWiFiClient] interface];
        if (!wifiInterface) {
            info.errorMessage = strdup(@"Unable to access Wi-Fi interface".UTF8String);
            return info;
        }

        // Collect all WiFi information
        // Note: On macOS Big Sur+, SSID and BSSID require location permission
        // Without permission, they will be empty strings (no error)
        info.rssi = (int)wifiInterface.rssiValue;
        info.ssid = strdup([wifiInterface.ssid UTF8String] ?: "");
        info.bssid = strdup([wifiInterface.bssid UTF8String] ?: "");
        info.channel = (int)wifiInterface.wlanChannel.channelNumber;
        info.noise = (int)wifiInterface.noiseMeasurement;
        info.transmitRate = wifiInterface.transmitRate;
        info.hardwareAddress = strdup([wifiInterface.hardwareAddress UTF8String] ?: "");
        info.activePHYMode = (int)wifiInterface.activePHYMode;
    }

    return info;
}

// Function to check if we have location permission (used by auto-request logic)
bool HasLocationPermission() {
    if (@available(macOS 11.0, *)) {
        @autoreleasepool {
            CLLocationManager *manager = [[CLLocationManager alloc] init];
            CLAuthorizationStatus status = manager.authorizationStatus;
            return (status == kCLAuthorizationStatusAuthorized ||
                    status == kCLAuthorizationStatusAuthorizedAlways);
        }
    }
    return false;
}

#import <Cocoa/Cocoa.h>

@interface LocationPermissionRequester : NSObject <NSApplicationDelegate, CLLocationManagerDelegate>
@property (strong) CLLocationManager *locationManager;
@property (assign) BOOL permissionReceived;
@end

@implementation LocationPermissionRequester

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    NSLog(@"🚀 Datadog Agent: Requesting location permission for WiFi monitoring...");
    NSLog(@"   This allows collection of WiFi SSID and BSSID information.");
    
    self.locationManager = [[CLLocationManager alloc] init];
    self.locationManager.delegate = self;
    self.permissionReceived = NO;
    
    [self.locationManager requestWhenInUseAuthorization];
    
    // Auto-exit after 30 seconds if no response
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 30 * NSEC_PER_SEC), 
                  dispatch_get_main_queue(), ^{
        if (!self.permissionReceived) {
            NSLog(@"⏱️ Permission request timeout - exiting");
            [NSApp terminate:nil];
        }
    });
}

- (void)locationManagerDidChangeAuthorization:(CLLocationManager *)manager {
    CLAuthorizationStatus status = manager.authorizationStatus;
    
    if (self.permissionReceived) {
        return;
    }
    self.permissionReceived = YES;
    
    switch (status) {
        case kCLAuthorizationStatusAuthorizedAlways:
            NSLog(@"✅ Location permission GRANTED!");
            NSLog(@"   WiFi SSID and BSSID will be collected by the WLAN check.");
            break;
        case kCLAuthorizationStatusDenied:
            NSLog(@"❌ Location permission DENIED");
            NSLog(@"   Only signal strength (RSSI), noise, and data rates will be collected.");
            break;
        case kCLAuthorizationStatusNotDetermined:
            return; // Still waiting
        default:
            NSLog(@"❓ Unexpected authorization status: %d", (int)status);
            break;
    }
    
    // Exit after 2 seconds
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 2 * NSEC_PER_SEC), 
                  dispatch_get_main_queue(), ^{
        [NSApp terminate:nil];
    });
}

@end

// C function callable from Go - shows GUI permission dialog
void RequestLocationPermissionGUI(void) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        [app setActivationPolicy:NSApplicationActivationPolicyAccessory];
        
        LocationPermissionRequester *delegate = [[LocationPermissionRequester alloc] init];
        [app setDelegate:delegate];
        
        [app run];
    }
}