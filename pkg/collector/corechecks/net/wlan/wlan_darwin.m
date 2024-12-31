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
@property (nonatomic, assign) BOOL isRunning;
@end

@implementation LocationManager

- (instancetype)init {
    self = [super init];
    if (self) {
        self.isRunning = YES;
        [self setupLocationServices];
    }
    return self;
}

- (void)setupLocationServices {
    self.locationManager = [[CLLocationManager alloc] init];
    self.locationManager.delegate = self;
    
    if (self.locationManager.authorizationStatus == kCLAuthorizationStatusNotDetermined) {
        [self.locationManager requestWhenIsUseAuthorization];
    }
}

#pragma mark - CLLocationManagerDelegate

- (void)locationManagerDidChangeAuthorization:(CLLocationManager *)manager {
    CLAuthorizationStatus status = manager.authorizationStatus;
    
    switch (status) {
        case kCLAuthorizationStatusAuthorizedAlways:
            NSLog(@"Location services authorized!");
            [self.locationManager startUpdatingLocation];
            break;
            
        case kCLAuthorizationStatusDenied:
            NSLog(@"Location services denied by user");
            self.isRunning = NO;
            break;
            
        case kCLAuthorizationStatusRestricted:
            NSLog(@"Location services restricted");
            self.isRunning = NO;
            break;
        default:
            NSLog(@"Location services status undetermined: %d", status);
            break;
    }
}

- (void)locationManager:(CLLocationManager *)manager didUpdateLocations:(NSArray<CLLocation *> *)locations {
    CLLocation *location = [locations lastObject];
    NSLog(@"Location updated: %@", location);
    [self printCurrentWiFiInfo];
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
    CWInterface *wifiInterface = [[CWWiFiClient sharedWiFiClient] interface];

    WiFiInfo info;

    info.rssi = (int)wifiInterface.rssiValue;
    info.ssid = [[wifiInterface ssid] UTF8String];
    info.bssid = [[wifiInterface bssid] UTF8String];
    info.channel = (int)wifiInterface.wlanChannel.channelNumber;
    info.noise = (int)wifiInterface.noiseMeasurement;
    info.transmitRate = wifiInterface.transmitRate;
    info.hardwareAddress = [[wifiInterface hardwareAddress] UTF8String];

    return info;
}
