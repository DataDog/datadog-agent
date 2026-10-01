param([Parameter(Mandatory = $true)][string]$Domain)

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class DomainControllerLocator
{
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct DOMAIN_CONTROLLER_INFO
    {
        [MarshalAs(UnmanagedType.LPWStr)] public string DomainControllerName;
        [MarshalAs(UnmanagedType.LPWStr)] public string DomainControllerAddress;
        public uint DomainControllerAddressType;
        public Guid DomainGuid;
        [MarshalAs(UnmanagedType.LPWStr)] public string DomainName;
        [MarshalAs(UnmanagedType.LPWStr)] public string DnsForestName;
        public uint Flags;
        [MarshalAs(UnmanagedType.LPWStr)] public string DcSiteName;
        [MarshalAs(UnmanagedType.LPWStr)] public string ClientSiteName;
    }

    public sealed class Result
    {
        public string DomainControllerName { get; set; }
        public uint Flags { get; set; }
        public string DcSiteName { get; set; }
    }

    [DllImport("Netapi32.dll", CharSet = CharSet.Unicode)]
    private static extern int DsGetDcName(
        string computerName,
        string domainName,
        IntPtr domainGuid,
        string siteName,
        uint flags,
        out IntPtr domainControllerInfo);

    [DllImport("Netapi32.dll")]
    private static extern int NetApiBufferFree(IntPtr buffer);

    public static Result Discover(string domainName, uint flags)
    {
        IntPtr infoPointer;
        int result = DsGetDcName(null, domainName, IntPtr.Zero, null, flags, out infoPointer);
        if (result != 0)
        {
            throw new System.ComponentModel.Win32Exception(result);
        }

        try
        {
            DOMAIN_CONTROLLER_INFO info = (DOMAIN_CONTROLLER_INFO)Marshal.PtrToStructure(
                infoPointer,
                typeof(DOMAIN_CONTROLLER_INFO));
            return new Result
            {
                DomainControllerName = info.DomainControllerName,
                Flags = info.Flags,
                DcSiteName = info.DcSiteName,
            };
        }
        finally
        {
            NetApiBufferFree(infoPointer);
        }
    }
}
'@

Import-Module ActiveDirectory
$localController = Get-ADDomainController -Identity $env:COMPUTERNAME -ErrorAction Stop

# DS_AVOID_SELF observes the remote writable controller without changing DNS. This simulates
# the customer locator result; it does not claim that ordinary discovery always avoids an RODC.
$remoteController = [DomainControllerLocator]::Discover($Domain, 0x00004001)
[PSCustomObject]@{
    DomainControllerName = $remoteController.DomainControllerName
    Flags                = $remoteController.Flags
    LocalIsReadOnly      = $localController.IsReadOnly
    LocalSite            = $localController.Site
    RemoteSite           = $remoteController.DcSiteName
} | ConvertTo-Json -Compress
