#pragma once

class TargetMachine
{
private:
    DWORD  _serverType;
    std::wstring _machineName;
    std::wstring _domain;
    bool _isDomainJoined;

    DWORD DetectMachineType();
    bool DetectComputerName(COMPUTER_NAME_FORMAT fmt, std::wstring& result);
    DWORD DetectDomainInformation();
public:
    TargetMachine();
    TargetMachine(const TargetMachine&) = default;
    ~TargetMachine();

    DWORD Detect();

    std::wstring GetMachineName() const;
    std::wstring GetDomain() const;
    bool IsDomainJoined() const;
    bool IsServer() const;
    bool IsDomainController() const;
    bool IsBackupDomainController() const;
};
