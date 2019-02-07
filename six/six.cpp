// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.
#include <iostream>

#include <six.h>

const std::string &Six::getExtensionModuleName(Six::ExtensionModule m) {
    switch (m) {
    case DATADOG_AGENT:
        return _module_datadog_agent;
    default:
        return _module_unknown;
    }
}
