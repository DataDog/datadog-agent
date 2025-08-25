#import <Foundation/Foundation.h>
#import <CoreLocation/CoreLocation.h>
#import <CoreWLAN/CoreWLAN.h>
#import "wlan_darwin.h"

@interface LocationManager : NSObject 

@property (nonatomic, strong) CLLocationManager *locationManager;
@property (nonatomic, assign) BOOL isAuthorized;

- (instancetype)init;
- (BOOL)checkLocationPermissions;

@end

@implementation LocationManager

- (instancetype)init {
    self = [super init];
    if (self) {
        self.locationManager = [[CLLocationManager alloc] init];

        if (@available(macOS 11.0, *)) {
            CLAuthorizationStatus status = self.locationManager.authorizationStatus;
            if (status == kCLAuthorizationStatusNotDetermined) {
                [self.locationManager requestWhenInUseAuthorization];
            }
        }
    }
    return self;
}

- (BOOL)checkLocationPermissions {
    if (@available(macOS 11.0, *)) {
        CLAuthorizationStatus status = self.locationManager.authorizationStatus;
        return (status == kCLAuthorizationStatusAuthorized ||
                             status == kCLAuthorizationStatusAuthorizedAlways);
    }
    return NO;
}

@end

WiFiInfo GetWiFiInformation() {
    WiFiInfo info = {0};

    @autoreleasepool {
        LocationManager *manager = [[LocationManager alloc] init];
        if (![manager checkLocationPermissions]) {
            info.errorMessage = strdup(@"Location authorization not granted".UTF8String);
            return info;
        }

        CWInterface *wifiInterface = [[CWWiFiClient sharedWiFiClient] interface];
        if (!wifiInterface) {
            info.errorMessage = strdup(@"Unable to access Wi-Fi interface".UTF8String);
            return info;
        }

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