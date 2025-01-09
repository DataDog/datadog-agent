// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#include <Foundation/Foundation.h>
#include <CoreWLAN/CoreWLAN.h>
#include <CoreLocation/CoreLocation.h>
#include "wlan_darwin.h"


@interface LocationManager : NSObject <CLLocationManagerDelegate>
@property (nonatomic, strong) CLLocationManager *locationManager;
@end

@implementation LocationManager

- (instancetype)init {
    self = [super init];
    if (self) {
        [self setupLocationServices];
    }
    return self;
}

- (void)setupLocationServices {
    self.locationManager = [[CLLocationManager alloc] init];
    self.locationManager.delegate = self;
    
    if (@available(macOS 11.0, *)) {
        if (self.locationManager.authorizationStatus == kCLAuthorizationStatusNotDetermined) {
            [self.locationManager requestWhenInUseAuthorization];
        }
    } else {
        NSLog(@"Location services not available on this OS version");
    }
}

#pragma mark - CLLocationManagerDelegate

- (void)locationManagerDidChangeAuthorization:(CLLocationManager *)manager {
    if (@available(macOS 11.0, *)) {
        CLAuthorizationStatus status = manager.authorizationStatus;
        
        switch (status) {
            case kCLAuthorizationStatusAuthorizedAlways:
                NSLog(@"Location services authorized!");
                [self.locationManager startUpdatingLocation];
                break;
                
            case kCLAuthorizationStatusDenied:
                NSLog(@"Location services denied by user");
                break;
                
            case kCLAuthorizationStatusRestricted:
                NSLog(@"Location services restricted");
                break;
            default:
                NSLog(@"Location services status undetermined: %d", status);
                break;
        }
    } else {
        NSLog(@"Location services not available on this OS version");
    }
}

- (void)locationManager:(CLLocationManager *)manager didUpdateLocations:(NSArray<CLLocation *> *)locations {
    CLLocation *location = [locations lastObject];
    NSLog(@"Location updated: %@", location);
}

- (void)locationManager:(CLLocationManager *)manager didFailWithError:(NSError *)error {
    NSLog(@"Location update failed with error: %@", error.localizedDescription);
}

@end

// Wrapper function to start location updates
void InitLocationServices() {
    LocationManager *locationManager = [[LocationManager alloc] init];
}

// GetWiFiInformation return wifi data
WiFiInfo GetWiFiInformation() {
    @autoreleasepool {
        CWInterface *wifiInterface = [[CWWiFiClient sharedWiFiClient] interface];

        WiFiInfo info;

        info.rssi = (int)wifiInterface.rssiValue;
        info.ssid = strdup([[wifiInterface ssid] UTF8String]);
        info.bssid = strdup([[wifiInterface bssid] UTF8String]);
        info.channel = (int)wifiInterface.wlanChannel.channelNumber;
        info.noise = (int)wifiInterface.noiseMeasurement;
        info.transmitRate = wifiInterface.transmitRate;
        info.hardwareAddress = strdup([[wifiInterface hardwareAddress] UTF8String]);
        info.activePHYMode = (int)wifiInterface.activePHYMode;
        
        return info;
    }
}
