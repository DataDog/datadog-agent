// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef TIMESTAMPS_DARWIN_H
#define TIMESTAMPS_DARWIN_H

// Returns Unix timestamp (seconds since epoch) or 0 on error
// fileVaultEnabled: 1 = FileVault enabled, 0 = FileVault disabled
// errorOut: if non-NULL, set to a malloc'd error string on failure (caller must free)
double queryLoginWindowTimestamp(int fileVaultEnabled, char **errorOut);

// Returns Unix timestamp (seconds since epoch) or 0 on error
// errorOut: if non-NULL, set to a malloc'd error string on failure (caller must free)
double queryLoginTimestamp(char **errorOut);

// Returns Unix timestamp (seconds since epoch) or 0 on error
// errorOut: if non-NULL, set to a malloc'd error string on failure (caller must free)
double queryDesktopReadyTimestamp(char **errorOut);

// Returns 1 if FileVault is enabled, 0 if disabled, -1 on error
// errorOut: if non-NULL, set to a malloc'd error string on failure (caller must free)
int    checkFileVaultEnabled(char **errorOut);

#endif
