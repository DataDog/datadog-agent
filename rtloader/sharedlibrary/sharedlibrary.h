// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.
#ifndef DATADOG_AGENT_RTLOADER_SHARED_LIBRARY_H
#define DATADOG_AGENT_RTLOADER_SHARED_LIBRARY_H

#include <string>
#include <vector>

#include <rtloader.h>

class SharedLibrary : public RtLoader
{
public:
    SharedLibrary();
    ~SharedLibrary();

    bool runCheck(const char *checkName);
};

#endif