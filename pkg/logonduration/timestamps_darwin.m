// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#import <Foundation/Foundation.h>
#import <OSLog/OSLog.h>
#include <string.h>
#import "timestamps_darwin.h"

// queryLogTimestamp is a helper that queries the unified log for the first entry matching the predicate.
// Searches from the start of the current boot (equivalent to `log show --last boot`).
// Returns Unix timestamp (seconds since epoch) or 0 on error.
static double queryLogTimestamp(NSPredicate *predicate, const char *queryName, char **errorOut) {
    NSError *error = nil;

    OSLogStore *store = [OSLogStore localStoreAndReturnError:&error];
    if (store == nil) {
        if (errorOut) {
            NSString *msg = [NSString stringWithFormat:@"failed to open OSLogStore for %s: %@", queryName, [error localizedDescription]];
            *errorOut = strdup([msg UTF8String]);
        }
        return 0;
    }

    // Get position at the start of the current boot (equivalent to --last boot)
    OSLogPosition *position = [store positionWithTimeIntervalSinceLatestBoot:0];
    if (position == nil) {
        if (errorOut) {
            NSString *msg = [NSString stringWithFormat:@"failed to create log position for %s", queryName];
            *errorOut = strdup([msg UTF8String]);
        }
        return 0;
    }

    OSLogEnumerator *enumerator = [store entriesEnumeratorWithOptions:0
                                                             position:position
                                                            predicate:predicate
                                                                error:&error];
    if (enumerator == nil) {
        if (errorOut) {
            NSString *msg = [NSString stringWithFormat:@"failed to create log enumerator for %s: %@", queryName, [error localizedDescription]];
            *errorOut = strdup([msg UTF8String]);
        }
        return 0;
    }

    OSLogEntryLog *entry = [enumerator nextObject];
    if (entry == nil) {
        if (errorOut) {
            NSString *msg = [NSString stringWithFormat:@"no log entry found for %s after boot time", queryName];
            *errorOut = strdup([msg UTF8String]);
        }
        return 0;
    }

    NSDate *timestamp = entry.date;
    if (timestamp == nil) {
        if (errorOut) {
            NSString *msg = [NSString stringWithFormat:@"log entry for %s has no timestamp", queryName];
            *errorOut = strdup([msg UTF8String]);
        }
        return 0;
    }

    return [timestamp timeIntervalSince1970];
}

// queryLoginWindowTimestamp queries the unified log for when the login window appeared.
// fileVaultEnabled: 1 = FileVault enabled, 0 = FileVault disabled
// Returns Unix timestamp (seconds since epoch) or 0 on error.
double queryLoginWindowTimestamp(int fileVaultEnabled, char **errorOut) {
    @autoreleasepool {
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

        return queryLogTimestamp(predicate, queryName, errorOut);
    }
}

// queryLoginTimestamp queries the unified log for when the user completed login.
// This works the same way with or without FileVault.
// Returns Unix timestamp (seconds since epoch) or 0 on error.
double queryLoginTimestamp(char **errorOut) {
    @autoreleasepool {
        NSPredicate *predicate = [NSPredicate predicateWithFormat:
            @"process == 'SecurityAgent' AND composedMessage CONTAINS 'loginwindow:success is being invoked'"];

        return queryLogTimestamp(predicate, "login timestamp", errorOut);
    }
}

// queryDesktopReadyTimestamp queries the unified log for when the Dock checked in with launchservicesd.
// This indicates the desktop is ready for user interaction.
// Returns Unix timestamp (seconds since epoch) or 0 on error.
double queryDesktopReadyTimestamp(char **errorOut) {
    @autoreleasepool {
        // Equivalent to: log show --predicate '(process == "launchservicesd"
        //   AND (subsystem == "com.apple.launchservices" OR subsystem == "com.apple.launchservices:cas")
        //   AND eventMessage CONTAINS[c] "checkin"
        //   AND eventMessage CONTAINS[c] "com.apple.dock")'
        NSPredicate *predicate = [NSPredicate predicateWithFormat:
            @"process == 'launchservicesd' AND "
            "(subsystem == 'com.apple.launchservices' OR subsystem == 'com.apple.launchservices:cas') AND "
            "composedMessage CONTAINS[c] 'checkin' AND "
            "composedMessage CONTAINS[c] 'com.apple.dock'"];

        return queryLogTimestamp(predicate, "Dock checkin (desktop ready)", errorOut);
    }
}

// checkFileVaultEnabled checks if FileVault is enabled using fdesetup.
// Returns 1 if enabled, 0 if disabled, -1 on error.
int checkFileVaultEnabled(char **errorOut) {
    @autoreleasepool {
        NSTask *task = [[NSTask alloc] init];
        [task setLaunchPath:@"/usr/bin/fdesetup"];
        [task setArguments:@[@"status"]];

        NSPipe *pipe = [NSPipe pipe];
        [task setStandardOutput:pipe];
        [task setStandardError:pipe];

        NSError *error = nil;
        if (![task launchAndReturnError:&error]) {
            if (errorOut) {
                *errorOut = strdup([[error localizedDescription] UTF8String]);
            }
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

        if (errorOut) {
            *errorOut = strdup("unexpected fdesetup output");
        }
        return -1;
    }
}
