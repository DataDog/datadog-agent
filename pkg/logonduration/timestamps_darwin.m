// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#import <Foundation/Foundation.h>
#import <OSLog/OSLog.h>

// queryLoginTimestamp queries the unified log for login-related timestamps.
// queryType: 0 = login window time (WindowServer starting), 1 = login time (sessionDidLogin)
// Returns Unix timestamp (seconds since epoch) or 0 on error.
double queryLoginTimestamp(double bootTimestamp, int queryType) {
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
        
        if (queryType == 1) {
            predicate = [NSPredicate predicateWithFormat:
                @"process == 'loginwindow' AND composedMessage CONTAINS 'com.apple.sessionDidLogin'"];
            queryName = "sessionDidLogin";
        } else {
            predicate = [NSPredicate predicateWithFormat:
                @"process == 'WindowServer' AND composedMessage CONTAINS 'Server is starting up'"];
            queryName = "WindowServer 'Server is starting up'";
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
