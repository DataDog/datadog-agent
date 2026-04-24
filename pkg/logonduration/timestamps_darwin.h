// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef TIMESTAMPS_DARWIN_H
#define TIMESTAMPS_DARWIN_H

// Queries the login window timestamp. Sets *result to the Unix timestamp (seconds since epoch).
// fileVaultEnabled: 1 = FileVault enabled, 0 = FileVault disabled
// Returns NULL on success, or a malloc'd error string on failure (caller must free).
char *queryLoginWindowTimestamp(int fileVaultEnabled, double *result) __attribute__((warn_unused_result));

// Queries the login timestamp. Sets *result to the Unix timestamp (seconds since epoch).
// Returns NULL on success, or a malloc'd error string on failure (caller must free).
char *queryLoginTimestamp(double *result) __attribute__((warn_unused_result));

// Queries the desktop ready timestamp. Sets *result to the Unix timestamp (seconds since epoch).
// Returns NULL on success, or a malloc'd error string on failure (caller must free).
char *queryDesktopReadyTimestamp(double *result) __attribute__((warn_unused_result));

// Checks if FileVault is enabled. Sets *result to 1 if enabled, 0 if disabled.
// Returns NULL on success, or a malloc'd error string on failure (caller must free).
char *checkFileVaultEnabled(int *result) __attribute__((warn_unused_result));

#endif
