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
// Sets *result to the Unix timestamp (seconds since epoch) on success.
// Returns NULL on success, or a malloc'd error string on failure (caller must free).
static char *queryLogTimestamp(NSPredicate *predicate, const char *queryName, double *result) {
    NSError *error = nil;

    OSLogStore *store = [OSLogStore localStoreAndReturnError:&error];
    if (store == nil) {
        NSString *msg = [NSString stringWithFormat:@"failed to open OSLogStore for %s: %@", queryName, [error localizedDescription]];
        return strdup([msg UTF8String]);
    }

    // Get position at the start of the current boot (equivalent to --last boot)
    OSLogPosition *position = [store positionWithTimeIntervalSinceLatestBoot:0];
    if (position == nil) {
        NSString *msg = [NSString stringWithFormat:@"failed to create log position for %s", queryName];
        return strdup([msg UTF8String]);
    }

    OSLogEnumerator *enumerator = [store entriesEnumeratorWithOptions:0
                                                             position:position
                                                            predicate:predicate
                                                                error:&error];
    if (enumerator == nil) {
        NSString *msg = [NSString stringWithFormat:@"failed to create log enumerator for %s: %@", queryName, [error localizedDescription]];
        return strdup([msg UTF8String]);
    }

    OSLogEntryLog *entry = [enumerator nextObject];
    if (entry == nil) {
        NSString *msg = [NSString stringWithFormat:@"no log entry found for %s after boot time", queryName];
        return strdup([msg UTF8String]);
    }

    NSDate *timestamp = entry.date;
    if (timestamp == nil) {
        NSString *msg = [NSString stringWithFormat:@"log entry for %s has no timestamp", queryName];
        return strdup([msg UTF8String]);
    }

    *result = [timestamp timeIntervalSince1970];
    return NULL;
}

// queryLoginWindowTimestamp queries the unified log for when the login window appeared.
// fileVaultEnabled: 1 = FileVault enabled, 0 = FileVault disabled
// Sets *result to the Unix timestamp (seconds since epoch) on success.
// Returns NULL on success, or a malloc'd error string on failure (caller must free).
char *queryLoginWindowTimestamp(int fileVaultEnabled, double *result) {
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

        return queryLogTimestamp(predicate, queryName, result);
    }
}

// queryLoginTimestamp queries the unified log for when the user completed login.
// This works the same way with or without FileVault.
// Sets *result to the Unix timestamp (seconds since epoch) on success.
// Returns NULL on success, or a malloc'd error string on failure (caller must free).
char *queryLoginTimestamp(double *result) {
    @autoreleasepool {
        NSPredicate *predicate = [NSPredicate predicateWithFormat:
            @"process == 'SecurityAgent' AND composedMessage CONTAINS 'loginwindow:success is being invoked'"];

        return queryLogTimestamp(predicate, "login timestamp", result);
    }
}

// queryDesktopReadyTimestamp queries the unified log for when the Dock checked in with launchservicesd.
// This indicates the desktop is ready for user interaction.
// Sets *result to the Unix timestamp (seconds since epoch) on success.
// Returns NULL on success, or a malloc'd error string on failure (caller must free).
char *queryDesktopReadyTimestamp(double *result) {
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

        return queryLogTimestamp(predicate, "Dock checkin (desktop ready)", result);
    }
}

// checkFileVaultEnabled checks if FileVault is enabled using fdesetup.
// Sets *result to 1 if enabled, 0 if disabled.
// Returns NULL on success, or a malloc'd error string on failure (caller must free).
char *checkFileVaultEnabled(int *result) {
    @autoreleasepool {
        NSTask *task = [[NSTask alloc] init];
        [task setLaunchPath:@"/usr/bin/fdesetup"];
        [task setArguments:@[@"status"]];

        NSPipe *pipe = [NSPipe pipe];
        [task setStandardOutput:pipe];
        [task setStandardError:pipe];

        NSError *error = nil;
        if (![task launchAndReturnError:&error]) {
            return strdup([[error localizedDescription] UTF8String]);
        }

        [task waitUntilExit];

        NSData *data = [[pipe fileHandleForReading] readDataToEndOfFile];
        NSString *output = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];

        if ([output containsString:@"FileVault is On"]) {
            *result = 1;
            return NULL;
        } else if ([output containsString:@"FileVault is Off"]) {
            *result = 0;
            return NULL;
        }

        return strdup("unexpected fdesetup output");
    }
}
