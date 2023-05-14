// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef DD_EVENT_H
#define DD_EVENT_H

#include <windows.h>
#include <winerror.h>
#include "winevt.h"
#include <conio.h>
#include <stdio.h>
#include <stdlib.h>

typedef struct RichEvent_t {
    LPWSTR message;
    LPWSTR task;
    LPWSTR opcode;
    LPWSTR level;
} RichEvent ;

ULONGLONG startEventSubscribe(char *channel, char* query, ULONGLONG  ullBookmark, int flags, PVOID ctx);
RichEvent* EnrichEvent(ULONGLONG ullEvent);

#endif /* DD_EVENT_H */
