#import <Foundation/Foundation.h>
#import <CoreLocation/CoreLocation.h>
#import <CoreWLAN/CoreWLAN.h>
#import "wlan_darwin.h"

@interface LocationManager : NSObject <CLLocationManagerDelegate>

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
        self.locationManager.delegate = self;
        self.isAuthorized = NO;

        if (@available(macOS 10.15, *)) {
            CLAuthorizationStatus status = [CLLocationManager authorizationStatus];
            if (status == kCLAuthorizationStatusNotDetermined) {
                [self.locationManager requestWhenInUseAuthorization];
            }
        }
    }
    return self;
}

- (BOOL)checkLocationPermissions {
    if (@available(macOS 10.15, *)) {
        CLAuthorizationStatus status = [CLLocationManager authorizationStatus];
        self.isAuthorized = (status == kCLAuthorizationStatusAuthorized ||
                             status == kCLAuthorizationStatusAuthorizedAlways);
    }
    return self.isAuthorized;
}

@end

WiFiInfo GetWiFiInformation() {
    WiFiInfo info = {0};

    @autoreleasepool {
        LocationManager *manager = [[LocationManager alloc] init];
        if (![manager checkLocationPermissions]) {
            info.errorMessage = "Location authorization not granted";
            return info;
        }

        CWInterface *wifiInterface = [[CWWiFiClient sharedWiFiClient] interface];
        if (!wifiInterface) {
            info.errorMessage = "Unable to access Wi-Fi interface";
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