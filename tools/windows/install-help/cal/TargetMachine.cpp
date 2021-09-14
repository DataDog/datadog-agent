#include "stdafx.h"
#include "TargetMachine.h"
#include <DsGetDC.h>

TargetMachine::TargetMachine()
    : _serverType(0)
    , _machineName(L"")
    , _joinedDomain(L"")
    , _isDomainJoined(false)
    , _dnsDomainName(L"")
{
}

TargetMachine::~TargetMachine()
{
}

DWORD TargetMachine::Detect()
{
    DWORD lastError = DetectMachineType();
    if (lastError != ERROR_SUCCESS)
    {
        return lastError;
    }

    wchar_t buf[MAX_COMPUTERNAME_LENGTH + 1];
    DWORD sz = MAX_COMPUTERNAME_LENGTH + 1;
    if (!GetComputerNameW(buf, &sz))
    {
        lastError = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Failed to get computername %d", lastError);
        return lastError;
    }

    _wcslwr_s(buf, MAX_COMPUTERNAME_LENGTH + 1);
    _machineName = buf;
    WcaLog(LOGMSG_STANDARD, "Computername is %S (%d)", _machineName.c_str(), sz);

    // get the computername again and compare, just to make sure
    std::wstring compare_computer;
    if (DetectComputerName(ComputerNameDnsHostname, compare_computer))
    {
        if (_machineName != compare_computer)
        {
            WcaLog(LOGMSG_STANDARD, "Got two different computer names %S %S", _machineName.c_str(),
                   compare_computer.c_str());
        }
    }
    else
    {
        lastError = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Failed to get ComputerNameDnsHostname %d", lastError);
        return lastError;
    }

    // Retrieves a NetBIOS or DNS name associated with the local computer.
    if (DetectComputerName(ComputerNameDnsDomain, _dnsDomainName))
    {
        // newer domains will look like DNS domains.  (i.e. domain.local)
        // just take the domain portion, which is all we're interested in.
        size_t pos = _dnsDomainName.find(L'.');
        if (pos != std::wstring::npos)
        {
            _dnsDomainName = _dnsDomainName.substr(0, pos);
        }
    }
    else
    {
        lastError = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Failed to get ComputerNameDnsDomain %d", lastError);
        return lastError;
    }

    lastError = DetectDomainInformation();

    return lastError;
}

DWORD TargetMachine::DetectMachineType()
{
    SERVER_INFO_101 *serverInfo;
    DWORD status = NetServerGetInfo(nullptr, 101, reinterpret_cast<LPBYTE *>(&serverInfo));
    if (status != NERR_Success)
    {
        if (status == NERR_ServerNotStarted || status == NERR_ServiceNotInstalled || status == NERR_WkstaNotStarted)
        {
            // NetServerGetInfo will fail if the Server service isn't running,
            // but in that case it's safe to assume we are a workstation.
            WcaLog(LOGMSG_STANDARD, "Failed to get server info: %S", FormatErrorMessage(status).c_str());
            WcaLog(LOGMSG_STANDARD, "Continuing assuming machine type is SV_TYPE_WORKSTATION.");
            _serverType = SV_TYPE_WORKSTATION;
            return ERROR_SUCCESS;
        }
        else
        {
            return status;
        }
    }
    _serverType = serverInfo->sv101_type;
    if (SV_TYPE_WORKSTATION & _serverType)
    {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_WORKSTATION");
    }
    if (SV_TYPE_SERVER & _serverType)
    {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_SERVER");
    }
    if (SV_TYPE_DOMAIN_CTRL & _serverType)
    {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_DOMAIN_CTRL");
    }
    if (SV_TYPE_DOMAIN_BAKCTRL & _serverType)
    {
        WcaLog(LOGMSG_STANDARD, "machine is type SV_TYPE_DOMAIN_BAKCTRL");
    }
    if (serverInfo != nullptr)
    {
        (void)NetApiBufferFree(serverInfo);
    }
    return ERROR_SUCCESS;
}

bool TargetMachine::DetectComputerName(COMPUTER_NAME_FORMAT fmt, std::wstring &result)
{
    wchar_t *buffer = nullptr;
    DWORD sz = 0;
    BOOL res = GetComputerNameExW(fmt, buffer, &sz);
    if (res)
    {
        // this should never succeed
        WcaLog(LOGMSG_STANDARD, "Unexpected.  Didn't get buffer size for computer name %d", static_cast<int>(fmt));
        return false;
    }
    DWORD err = GetLastError();
    if (ERROR_MORE_DATA != err)
    {
        WcaLog(LOGMSG_STANDARD, "Unable to get computername info %d", err);
        return false;
    }
    buffer = new wchar_t[sz + 1];
    sz = sz + 1;
    res = GetComputerNameExW(fmt, buffer, &sz);
    if (res)
    {
        _wcslwr_s(buffer, sz + 1);
        result = buffer;
    }
    else
    {
        err = GetLastError();
        WcaLog(LOGMSG_STANDARD, "Unable to get computername info %d", err);
    }

    delete[] buffer;

    return res;
}

DWORD TargetMachine::DetectDomainInformation()
{
    // check if it's actually domain joined or not
    LPWSTR name = nullptr;
    NETSETUP_JOIN_STATUS st;
    DWORD nErr = NetGetJoinInformation(nullptr, &name, &st);
    if (nErr == NERR_Success)
    {
        _joinedDomain = name;
        (void)NetApiBufferFree(name);
    }
    else
    {
        /*
         * If the function fails, the return value can be the following error code or one of the system error codes.
         *
         * - ERROR_NOT_ENOUGH_MEMORY
         * Not enough storage is available to process this command.
         */
        WcaLog(LOGMSG_STANDARD, "Error getting domain joining information %d %d", nErr, GetLastError());
        return nErr;
    }

    switch (st)
    {
    case NetSetupUnknownStatus:
        WcaLog(LOGMSG_STANDARD, "Unknown domain joining status, assuming not joined");
        break;
    case NetSetupUnjoined:
        WcaLog(LOGMSG_STANDARD, "Computer explicitly not joined to domain");
        break;
    case NetSetupWorkgroupName:
        WcaLog(LOGMSG_STANDARD, "Computer is joined to a workgroup");
        break;
    case NetSetupDomainName:
        // Print both domain names: NETBIOS and FQDN
        WcaLog(LOGMSG_STANDARD, "Computer is joined to domain \"%S\" (\"%S\")", _joinedDomain.c_str(), _dnsDomainName.c_str());
        _isDomainJoined = true;
        break;
    }

    if (_isDomainJoined)
    {
        // Detect if we are on a read-only domain controller
        if (IsDomainController())
        {
            PDOMAIN_CONTROLLER_INFO dcInfo;
            // See https://docs.microsoft.com/en-us/windows/win32/api/dsgetdc/nf-dsgetdc-dsgetdcnamea
            // ComputerName = nullptr means local computer
            nErr = DsGetDcName(
                /*ComputerName*/ nullptr, _joinedDomain.c_str(),
                /*DomainGuid*/ nullptr,
                /*SiteName*/ nullptr, 0, &dcInfo);
            if (nErr != ERROR_SUCCESS)
            {
                return nErr;
            }
            _dcFlags = dcInfo->Flags;
            WcaLog(LOGMSG_STANDARD, "Domain Controller is %S", IsReadOnlyDomainController() ? L"Read-Only" : L"Writable");
            NetApiBufferFree(dcInfo);
        }
    }

    return ERROR_SUCCESS;
}

std::wstring TargetMachine::GetMachineName() const
{
    return _machineName;
}

std::wstring TargetMachine::JoinedDomainName() const
{
    return _joinedDomain;
}

std::wstring TargetMachine::DnsDomainName() const
{
    return _dnsDomainName;
}

bool TargetMachine::IsDomainJoined() const
{
    return _isDomainJoined;
}

bool TargetMachine::IsServer() const
{
    return SV_TYPE_SERVER & _serverType;
}

bool TargetMachine::IsDomainController() const
{
    return IsBackupDomainController() || (SV_TYPE_DOMAIN_CTRL & _serverType);
}

bool TargetMachine::IsBackupDomainController() const
{
    return SV_TYPE_DOMAIN_BAKCTRL & _serverType;
}

bool TargetMachine::IsReadOnlyDomainController() const
{
    return IsDomainController() && !(_dcFlags & DS_WRITABLE_FLAG);
}
