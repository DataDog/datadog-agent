// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

#pragma once
#include <Windows.h>
#include <string>
#include <chrono>

class Service
{
private:
    Service(Service const&) = delete;

    SC_HANDLE _scManagerHandle;
    SC_HANDLE _serviceHandle;
    DWORD _processId;
public:
    Service(std::wstring const& name);
    Service(Service&&) = default;
    ~Service();

    DWORD PID();
    void Start(std::chrono::milliseconds timeout = std::chrono::seconds(60));
    void Stop(std::chrono::milliseconds timeout = std::chrono::seconds(30));
};

