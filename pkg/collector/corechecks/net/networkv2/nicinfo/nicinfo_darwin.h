// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef NICINFO_DARWIN_H
#define NICINFO_DARWIN_H

typedef struct {
    char *name;
    char *description;
    char *vendor;
    char *model;
    char *driverClass;
    char *driver;
    char *driverVersion;
    unsigned char mac[8];
    int macLength;
    // inRegistry is 1 when the IOKit registry has the interface, and builtin is only set then
    int inRegistry;
    int builtin;
} NICInfo;

// copyNICInfo returns an array of count interfaces, to free with freeNICInfo. It sets failed and
// returns NULL when IOKit or SystemConfiguration cannot list the interfaces.
NICInfo *copyNICInfo(int *count, int *failed);
// freeNICInfo frees the interfaces of copyNICInfo and their strings.
void freeNICInfo(NICInfo *infos, int count);

#endif
