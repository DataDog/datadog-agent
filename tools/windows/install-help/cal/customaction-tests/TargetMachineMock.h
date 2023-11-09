#pragma once

#include <strsafe.h>
#include <string>
#include <TargetMachine.h>
#include <gmock/gmock.h>

class TargetMachineMock : public ITargetMachine
{
public:
    MOCK_METHOD(DWORD,          Detect,                         (), (override));
    MOCK_METHOD(std::wstring,   GetMachineName,                 (), (const, override));
    MOCK_METHOD(std::wstring,   JoinedDomainName,               (), (const, override));
    MOCK_METHOD(std::wstring,   DnsDomainName,                  (), (const, override));
    MOCK_METHOD(bool,           IsDomainJoined,                 (), (const, override));
    MOCK_METHOD(bool,           IsServer,                       (), (const, override));
    MOCK_METHOD(bool,           IsDomainController,             (), (const, override));
    MOCK_METHOD(bool,           IsBackupDomainController,       (), (const, override));
    MOCK_METHOD(bool,           IsReadOnlyDomainController,     (), (const, override));
};
