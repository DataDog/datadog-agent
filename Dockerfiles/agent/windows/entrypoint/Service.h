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
    std::wstring _name;
    DWORD _processId;
public:
    Service(std::wstring const& name);
    Service(Service&&) = default;
    ~Service();

    DWORD PID();
    void Start(std::chrono::milliseconds timeout = std::chrono::seconds(30));
    void Stop(std::chrono::milliseconds timeout = std::chrono::seconds(30));
};

