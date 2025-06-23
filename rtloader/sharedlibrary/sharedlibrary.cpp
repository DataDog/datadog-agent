// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

#include "sharedlibrary.h"

SharedLibrary::SharedLibrary():
    RtLoader(nullptr) 
{
}

SharedLibrary::~SharedLibrary()
{
}

bool SharedLibrary::runCheck(const char *checkName)
{
    return false;
}
