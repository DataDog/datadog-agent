// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef TIMESTAMPS_DARWIN_H
#define TIMESTAMPS_DARWIN_H

// Returns Unix timestamp (seconds since epoch) or 0 on error
// fileVaultEnabled: 1 = FileVault enabled, 0 = FileVault disabled
double queryLoginWindowTimestamp(int fileVaultEnabled);

// Returns Unix timestamp (seconds since epoch) or 0 on error
double queryLoginTimestamp(void);

// Returns Unix timestamp (seconds since epoch) or 0 on error
double queryDesktopReadyTimestamp(void);

// Returns 1 if FileVault is enabled, 0 if disabled, -1 on error
int checkFileVaultEnabled(void);

#endif
