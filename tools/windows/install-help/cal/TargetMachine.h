#pragma once

class TargetMachine
{
private:
    DWORD _serverType;
    DWORD _dcFlags;
    std::wstring _machineName;
    std::wstring _joinedDomain;
    bool _isDomainJoined;
    std::wstring _dnsDomainName;

    DWORD DetectMachineType();
    bool DetectComputerName(COMPUTER_NAME_FORMAT fmt, std::wstring& result);
    DWORD DetectDomainInformation();
public:
    TargetMachine();
    TargetMachine(const TargetMachine&) = default;
    ~TargetMachine();

    DWORD Detect();

    /// <summary>
    /// Get the name of the computer.
    /// </summary>
    /// <returns>The name of the computer.</returns>
    std::wstring GetMachineName() const;

    /// <summary>
    /// Returns the name of the domain this computer is joined to.
    /// It should also match the pre-Windows 2000 name of the domain, which
    /// can be different from the DNS name of the domain returned by <see cref="DnsDomainName"/>
    ///
    /// For example the DNS domain "datadohq.com" can have a pre-Windows 2000
    /// name of "DDOG" and this method would return "DDOG".
    /// </summary>
    /// <returns>A wide string with the name of the domain this computer is joined to.</returns>
    std::wstring JoinedDomainName() const;

    /// <summary>
    /// Returns the DNS name of the domain this computer is joined to.
    /// It can be different from the pre-Windows 2000 domain name returned by <see cref="JoinedDomainName"/>.
    ///
    /// For example the DNS domain "datadohq.com" can have a pre-Windows 2000
    /// name of "DDOG" and this method would return "datadoghq.com".
    /// </summary>
    /// <remarks>
    /// When creating a user with the domain name returned by this method, the subsequent call to
    /// <see cref="LookupAccountName"/> can fail with code 1332 (NONE_MAPPED).
    /// </remarks>
    /// <returns>A wide string with the DNS name of the domain this computer is joined to.</returns>
    std::wstring DnsDomainName() const;

    /// <summary>
    /// Check if the computer is part of a domain or is a standalone machine.
    /// </summary>
    /// <returns>True if the computer is joined to a domain, false otherwise.</returns>
    bool IsDomainJoined() const;

    /// <summary>
    /// Check if the computer is a workstation or a server.
    /// </summary>
    /// <returns>True if the computer is a server, false otherwise.</returns>
    bool IsServer() const;

    /// <summary>
    /// Check if the computer is a domain controller.
    /// </summary>
    /// <returns>True if the computer is a domain controller, false otherwise.</returns>
    bool IsDomainController() const;

    /// <summary>
    /// Check if the computer is a backup domain controller.
    /// </summary>
    /// <returns>True if the computer is a domain controller, false otherwise.</returns>
    bool IsBackupDomainController() const;

    /// <summary>
    /// Chef if the computer is a read-only domain controller.
    /// </summary>
    /// <remarks>It is not possible to create users on a read-only domain controller.</remarks>
    /// <returns>True if the computer is a read-only domain controller.</returns>
    bool IsReadOnlyDomainController() const;
};
