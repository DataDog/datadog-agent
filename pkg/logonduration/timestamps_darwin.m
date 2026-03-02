// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#import <Foundation/Foundation.h>
#import <OSLog/OSLog.h>

// queryLoginWindowTimestamp queries the unified log for when the login window appeared.
// fileVaultEnabled: 1 = FileVault enabled, 0 = FileVault disabled
// Returns Unix timestamp (seconds since epoch) or 0 on error.
double queryLoginWindowTimestamp(double bootTimestamp, int fileVaultEnabled) {
    @autoreleasepool {
        NSError *error = nil;
        
        OSLogStore *store = [OSLogStore localStoreAndReturnError:&error];
        if (store == nil) {
            NSLog(@"logonduration: Failed to open OSLogStore: %@", error);
            return 0;
        }
        
        NSDate *startDate = [NSDate dateWithTimeIntervalSince1970:bootTimestamp];
        
        OSLogPosition *position = [store positionWithDate:startDate];
        if (position == nil) {
            NSLog(@"logonduration: Failed to create log position for boot time");
            return 0;
        }
        
        NSPredicate *predicate;
        const char *queryName;
        
        if (fileVaultEnabled) {
            predicate = [NSPredicate predicateWithFormat:
                @"(process == 'SecurityAgent' OR process == 'authorizationhost') AND composedMessage CONTAINS 'FVUnlock session: fvunlock'"];
            queryName = "FVUnlock session";
        } else {
            predicate = [NSPredicate predicateWithFormat:
                @"process == 'loginwindow' AND composedMessage CONTAINS 'Login Window Application Started'"];
            queryName = "Login Window Application Started";
        }
        
        OSLogEnumerator *enumerator = [store entriesEnumeratorWithOptions:0
                                                                 position:position
                                                                predicate:predicate
                                                                    error:&error];
        if (enumerator == nil) {
            NSLog(@"logonduration: Failed to create log enumerator for %s: %@", queryName, error);
            return 0;
        }
        
        OSLogEntryLog *entry = [enumerator nextObject];
        if (entry == nil) {
            NSLog(@"logonduration: No log entry found for %s after boot time", queryName);
            return 0;
        }
        
        NSDate *timestamp = entry.date;
        if (timestamp == nil) {
            NSLog(@"logonduration: Log entry for %s has no timestamp", queryName);
            return 0;
        }
        
        double result = [timestamp timeIntervalSince1970];
        NSLog(@"logonduration: Found %s at %@ (%.3f)", queryName, timestamp, result);
        return result;
    }
}

// queryLoginTimestamp queries the unified log for when the user completed login.
// This works the same way with or without FileVault.
// Returns Unix timestamp (seconds since epoch) or 0 on error.
double queryLoginTimestamp(double bootTimestamp) {
    @autoreleasepool {
        NSError *error = nil;
        
        OSLogStore *store = [OSLogStore localStoreAndReturnError:&error];
        if (store == nil) {
            NSLog(@"logonduration: Failed to open OSLogStore: %@", error);
            return 0;
        }
        
        NSDate *startDate = [NSDate dateWithTimeIntervalSince1970:bootTimestamp];
        
        OSLogPosition *position = [store positionWithDate:startDate];
        if (position == nil) {
            NSLog(@"logonduration: Failed to create log position for boot time");
            return 0;
        }
        
        NSPredicate *predicate = [NSPredicate predicateWithFormat:
            @"process == 'SecurityAgent' AND composedMessage CONTAINS 'loginwindow:success is being invoked'"];
        
        OSLogEnumerator *enumerator = [store entriesEnumeratorWithOptions:0
                                                                 position:position
                                                                predicate:predicate
                                                                    error:&error];
        if (enumerator == nil) {
            NSLog(@"logonduration: Failed to create log enumerator for login timestamp: %@", error);
            return 0;
        }
        
        OSLogEntryLog *entry = [enumerator nextObject];
        if (entry == nil) {
            NSLog(@"logonduration: No log entry found for login timestamp after boot time");
            return 0;
        }
        
        NSDate *timestamp = entry.date;
        if (timestamp == nil) {
            NSLog(@"logonduration: Log entry for login timestamp has no timestamp");
            return 0;
        }
        
        double result = [timestamp timeIntervalSince1970];
        NSLog(@"logonduration: Found login timestamp at %@ (%.3f)", timestamp, result);
        return result;
    }
}

// checkFileVaultEnabled checks if FileVault is enabled using fdesetup.
// Returns 1 if enabled, 0 if disabled, -1 on error.
int checkFileVaultEnabled(void) {
    @autoreleasepool {
        NSTask *task = [[NSTask alloc] init];
        [task setLaunchPath:@"/usr/bin/fdesetup"];
        [task setArguments:@[@"status"]];
        
        NSPipe *pipe = [NSPipe pipe];
        [task setStandardOutput:pipe];
        [task setStandardError:pipe];
        
        NSError *error = nil;
        if (![task launchAndReturnError:&error]) {
            NSLog(@"logonduration: Failed to run fdesetup: %@", error);
            return -1;
        }
        
        [task waitUntilExit];
        
        NSData *data = [[pipe fileHandleForReading] readDataToEndOfFile];
        NSString *output = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        
        if ([output containsString:@"FileVault is On"]) {
            return 1;
        } else if ([output containsString:@"FileVault is Off"]) {
            return 0;
        }
        
        NSLog(@"logonduration: Unexpected fdesetup output: %@", output);
        return -1;
    }
}

// queryDesktopReadyTimestamp queries the unified log for when the Dock checked in with launchservicesd.
// This indicates the desktop is ready for user interaction.
// Returns Unix timestamp (seconds since epoch) or 0 on error.
double queryDesktopReadyTimestamp(double bootTimestamp) {
    @autoreleasepool {
        NSError *error = nil;
        
        OSLogStore *store = [OSLogStore localStoreAndReturnError:&error];
        if (store == nil) {
            NSLog(@"logonduration: Failed to open OSLogStore for desktop ready: %@", error);
            return 0;
        }
        
        NSDate *startDate = [NSDate dateWithTimeIntervalSince1970:bootTimestamp];
        
        OSLogPosition *position = [store positionWithDate:startDate];
        if (position == nil) {
            NSLog(@"logonduration: Failed to create log position for boot time (desktop ready)");
            return 0;
        }
        
        // Query for Dock checkin with launchservicesd - indicates desktop is ready
        // Equivalent to: log show --predicate '(process == "launchservicesd"
        //   AND (subsystem == "com.apple.launchservices" OR subsystem == "com.apple.launchservices:cas")
        //   AND eventMessage CONTAINS[c] "checkin"
        //   AND eventMessage CONTAINS[c] "com.apple.dock")'
        NSPredicate *predicate = [NSPredicate predicateWithFormat:
            @"process == 'launchservicesd' AND "
            "(subsystem == 'com.apple.launchservices' OR subsystem == 'com.apple.launchservices:cas') AND "
            "composedMessage CONTAINS[c] 'checkin' AND "
            "composedMessage CONTAINS[c] 'com.apple.dock'"];
        
        OSLogEnumerator *enumerator = [store entriesEnumeratorWithOptions:0
                                                                 position:position
                                                                predicate:predicate
                                                                    error:&error];
        if (enumerator == nil) {
            NSLog(@"logonduration: Failed to create log enumerator for desktop ready: %@", error);
            return 0;
        }
        
        OSLogEntryLog *entry = [enumerator nextObject];
        if (entry == nil) {
            NSLog(@"logonduration: No log entry found for Dock checkin after boot time");
            return 0;
        }
        
        NSDate *timestamp = entry.date;
        if (timestamp == nil) {
            NSLog(@"logonduration: Log entry for Dock checkin has no timestamp");
            return 0;
        }
        
        double result = [timestamp timeIntervalSince1970];
        NSLog(@"logonduration: Found Dock checkin (desktop ready) at %@ (%.3f)", timestamp, result);
        return result;
    }
}
