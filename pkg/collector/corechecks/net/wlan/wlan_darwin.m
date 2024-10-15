// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#include <Foundation/Foundation.h>
#include <CoreWLAN/CoreWLAN.h>
#include <CoreLocation/CoreLocation.h>

// Use @interface to define a class that acts as a delegate for CLLocationManager
@interface LocationManager : NSObject <CLLocationManagerDelegate>
@property (strong, nonatomic) CLLocationManager *locationManager;
@end

@implementation LocationManager

- (instancetype)init {
    self = [super init];
    if (self) {
        self.locationManager = [[CLLocationManager alloc] init];
        self.locationManager.delegate = self;
        // macOS can use this method for requesting authorization
        [self.locationManager requestAlwaysAuthorization];
    }
    return self;
}

- (void)locationManager:(CLLocationManager *)manager didChangeAuthorizationStatus:(CLAuthorizationStatus)status {
    switch (status) {
        case kCLAuthorizationStatusAuthorized:
            NSLog(@"Location access granted");
            [self.locationManager startUpdatingLocation];
            break;
        case kCLAuthorizationStatusDenied:
        case kCLAuthorizationStatusRestricted:
            NSLog(@"Location access denied");
            break;
        default:
            break;
    }
}

- (void)locationManager:(CLLocationManager *)manager didUpdateLocations:(NSArray<CLLocation *> *)locations {
    CLLocation *location = [locations lastObject];
    NSLog(@"Latitude: %f, Longitude: %f", location.coordinate.latitude, location.coordinate.longitude);
}

@end

LocationManager *locationManager = nil;

// Wrapper function to initialize location manager
void InitLocationManager() {
    locationManager = [[LocationManager alloc] init];
}

// Wrapper function to start location updates
void StartLocationUpdates() {
    [locationManager.locationManager startUpdatingLocation];
}

// Wrapper function to stop location updates
void StopLocationUpdates() {
    [locationManager.locationManager stopUpdatingLocation];
}


// WiFiInfo struct to hold WiFi data
typedef struct {
    int rssi;
    const char *ssid;
    const char *bssid;
} WiFiInfo;

// GetWiFiInformation return wifi data
WiFiInfo GetWiFiInformation() {
    CWInterface *wifiInterface = [[CWWiFiClient sharedWiFiClient] interface];

    WiFiInfo info;

    info.rssi = (int)wifiInterface.rssiValue;
    info.ssid = [[wifiInterface ssid] UTF8String];
    info.bssid = [[wifiInterface bssid] UTF8String];

    return info;
}