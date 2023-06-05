using System;
using System.ComponentModel;
using System.DirectoryServices.ActiveDirectory;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Text;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Native
{
    /// <summary>
    /// See https://learn.microsoft.com/en-us/windows/win32/secauthz/account-rights-constants
    /// </summary>
    public enum AccountRightsConstants
    {
        SeDenyInteractiveLogonRight,
        SeDenyNetworkLogonRight,
        SeDenyRemoteInteractiveLogonRight,
        SeServiceLogonRight
    }

    public enum COMPUTER_NAME_FORMAT
    {
        ComputerNameNetBIOS,
        ComputerNameDnsHostname,
        ComputerNameDnsDomain,
        ComputerNameDnsFullyQualified,
        ComputerNamePhysicalNetBIOS,
        ComputerNamePhysicalDnsHostname,
        ComputerNamePhysicalDnsDomain,
        ComputerNamePhysicalDnsFullyQualified
    }
    public enum SID_NAME_USE
    {
        SidTypeUser = 1,
        SidTypeGroup,
        SidTypeDomain,
        SidTypeAlias,
        SidTypeWellKnownGroup,
        SidTypeDeletedAccount,
        SidTypeInvalid,
        SidTypeUnknown,
        SidTypeComputer
    }

    public class Win32NativeMethods : INativeMethods
    {
        #region Native methods
        private enum ReturnCodes
        {
            NO_ERROR = 0,
            ERROR_ACCESS_DENIED = 5,
            ERROR_INVALID_PARAMETER = 87,
            ERROR_INSUFFICIENT_BUFFER = 122,
            ERROR_BAD_ARGUMENTS = 160,
            ERROR_INVALID_FLAGS = 1004,
            ERROR_NOT_FOUND = 1168,
            ERROR_CANCELLED = 1223,
            ERROR_NO_SUCH_LOGON_SESSION = 1312,

            ERROR_INVALID_ACCOUNT_NAME = 1315,
            ERROR_NONE_MAPPED = 1332,

            // One or more of the members specified were already members of the local group. No new members were added.
            ERROR_MEMBER_IN_ALIAS = 1378,

            // One or more of the members specified do not exist. Therefore, no new members were added.
            ERROR_NO_SUCH_MEMBER = 1387,

            // One or more of the members cannot be added because their account type is invalid. No new members were added.
            ERROR_INVALID_MEMBER = 1388,

            //The local group specified by the groupname parameter does not exist.
            NERR_GROUP_NOT_FOUND = 2220,
        }

        private enum NtStatus : uint
        {
            Success = 0x00000000
        }

        public static int SERVICE_NO_CHANGE = -1;

        [DllImport("logoncli.dll", CharSet = CharSet.Unicode)]
        private static extern NtStatus NetIsServiceAccount(
            string serverName,
            string accountName,
            out bool isService);

        [DllImport("advapi32.dll", CharSet = CharSet.Auto, SetLastError = true)]
        private static extern bool LookupAccountName(
            string lpSystemName,
            string lpAccountName,
            [MarshalAs(UnmanagedType.LPArray)] byte[] sid,
            ref uint cbSid,
            StringBuilder referencedDomainName,
            ref uint cchReferencedDomainName,
            out SID_NAME_USE peUse);

        [DllImport("advapi32.dll", CharSet = CharSet.Auto, SetLastError = true)]
        private static extern bool LookupAccountSid(
            string lpSystemName,
            [MarshalAs(UnmanagedType.LPArray)] byte[] sid,
            StringBuilder lpName,
            ref uint cchName,
            StringBuilder referencedDomainName,
            ref uint cchReferencedDomainName,
            out SID_NAME_USE peUse);

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        private struct USER_INFO_1
        {
            [MarshalAs(UnmanagedType.LPWStr)]
            public string sUsername;

            [MarshalAs(UnmanagedType.LPWStr)]
            public string sPassword;

            [MarshalAs(UnmanagedType.U4)]
            public UserFlags uiPasswordAge;

            [MarshalAs(UnmanagedType.U4)]
            public uint uiPriv;

            [MarshalAs(UnmanagedType.LPWStr)]
            public string sHome_Dir;

            [MarshalAs(UnmanagedType.LPWStr)]
            public string sComment;

            [MarshalAs(UnmanagedType.U4)]
            public UserFlags uiFlags;

            [MarshalAs(UnmanagedType.LPWStr)]
            public string sScript_Path;
        }

        private const uint USER_PRIV_USER = 1;

        [Flags]
        private enum UserFlags : uint
        {
            UF_DONT_EXPIRE_PASSWD = 0x10000
        }

        [DllImport("netapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern int NetUserAdd(
            [MarshalAs(UnmanagedType.LPWStr)] string servername,
            uint level,
            ref USER_INFO_1 userinfo,
            out uint parm_err);

        public int AddUser(string userName, string password)
        {
            var userInfo = new USER_INFO_1
            {
                sComment = "User context under which the DatadogAgent service runs",
                sUsername = userName,
                sPassword = password,
                uiPriv = USER_PRIV_USER,
                uiFlags = UserFlags.UF_DONT_EXPIRE_PASSWD
            };
            return NetUserAdd(null, 1, ref userInfo, out _);
        }

        [StructLayout(LayoutKind.Sequential)]
        private struct LOCALGROUP_MEMBERS_INFO_0
        {
            [MarshalAs(UnmanagedType.SysInt)]
            public IntPtr pSID;
        }

        [DllImport("NetApi32.dll", CharSet = CharSet.Auto, SetLastError = true)]
        private static extern int NetLocalGroupAddMembers(
            string servername, //server name
            string groupname, //group name
            uint level, //info level
            ref LOCALGROUP_MEMBERS_INFO_0 buf, //Group info structure
            uint totalentries //number of entries
        );

        [DllImport("netapi32.dll", CharSet = CharSet.Unicode)]
        public static extern int NetUserSetInfo(
            [MarshalAs(UnmanagedType.LPWStr)] string servername,
            string username,
            int level,
            ref USER_INFO_1003 buf,
            out uint parm_err
        );

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        public struct USER_INFO_1003
        {
            public string sPassword;
        }

        [Flags]
        public enum ServerTypes : uint
        {
            DomainCtrl = 0x00000008,
            BackupDomainCtrl = 0x00000010,
        };

        public enum ServerPlatform
        {
        }

        [StructLayout(LayoutKind.Sequential)]
        public struct SERVER_INFO_101
        {
            public ServerPlatform PlatformId;
            [MarshalAs(UnmanagedType.LPWStr)]
            public string Name;
            public int VersionMajor;
            public int VersionMinor;
            public ServerTypes Type;
            [MarshalAs(UnmanagedType.LPWStr)]
            public string Comment;
        }

        [DllImport("Netapi32", CharSet = CharSet.Auto, SetLastError = true)]
        private static extern int NetServerGetInfo(string serverName, int level, out IntPtr pSERVER_INFO_XXX);
        [DllImport("Netapi32", CharSet = CharSet.Auto, SetLastError = true)]
        private static extern int NetApiBufferFree(IntPtr Buffer);

        // Wrapper function to allow passing of structure and auto-find of level if structure contains it.
        public static T NetServerGetInfo<T>(string serverName = null, int level = 0) where T : struct
        {
            if (level == 0)
            {
               level = int.Parse(System.Text.RegularExpressions.Regex.Replace(typeof(T).Name, @"[^\d]", ""));
            }
            var ptr = IntPtr.Zero;
            try
            {
                var ret = NetServerGetInfo(serverName, level, out ptr);
                if (ret != 0)
                {
                    throw new System.ComponentModel.Win32Exception(ret);
                }
                return (T)Marshal.PtrToStructure(ptr, typeof(T));
            }
            finally
            {
                if (ptr != IntPtr.Zero)
                {
                    NetApiBufferFree(ptr);
                }
            }
        }

        /// <summary>Receives a security identifier (SID) and returns a SID representing the domain of that SID.</summary>
        /// <param name="pSid">A pointer to the SID to examine.</param>
        /// <param name="pDomainSid">Pointer that <b>GetWindowsAccountDomainSid</b> fills with a pointer to a SID representing the domain.</param>
        /// <param name="cbDomainSid">A pointer to a <b>DWORD</b> that <b>GetWindowsAccountDomainSid</b> fills with the size of the domain SID, in bytes.</param>
        /// <returns>
        /// <para>Returns <b>TRUE</b> if successful. Otherwise, returns <b>FALSE</b>. For extended error information, call <a href="/windows/desktop/api/errhandlingapi/nf-errhandlingapi-getlasterror">GetLastError</a>.</para>
        /// </returns>
        /// <remarks>
        /// <para><see href="https://docs.microsoft.com/windows/win32/api//securitybaseapi/nf-securitybaseapi-getwindowsaccountdomainsid">Learn more about this API from docs.microsoft.com</see>.</para>
        /// </remarks>
        [DllImport("ADVAPI32.dll", ExactSpelling = true, SetLastError = true)]
        [DefaultDllImportSearchPaths(DllImportSearchPath.System32)]
        public static extern bool GetWindowsAccountDomainSid(
            [MarshalAs(UnmanagedType.LPArray)] byte[] pSid,
            [MarshalAs(UnmanagedType.LPArray)] byte[] pDomainSid,
            ref uint cbDomainSid);

        [DllImport("advapi32.dll", PreserveSig = true)]
        private static extern uint LsaOpenPolicy(
            ref LSA_UNICODE_STRING SystemName,
            ref LSA_OBJECT_ATTRIBUTES ObjectAttributes,
            int DesiredAccess,
            out IntPtr PolicyHandle);

        [DllImport("advapi32.dll", SetLastError = true, PreserveSig = true)]
        private static extern uint LsaAddAccountRights(
            IntPtr PolicyHandle, IntPtr AccountSid,
            LSA_UNICODE_STRING[] UserRights,
            long CountOfRights);

        [StructLayout(LayoutKind.Sequential)]
        private struct LSA_UNICODE_STRING
        {
            public ushort Length;
            public ushort MaximumLength;
            public IntPtr Buffer;
        }

        [StructLayout(LayoutKind.Sequential)]
        private struct LSA_OBJECT_ATTRIBUTES
        {
            public int Length;
            public IntPtr RootDirectory;
            public readonly LSA_UNICODE_STRING ObjectName;
            public uint Attributes;
            public IntPtr SecurityDescriptor;
            public IntPtr SecurityQualityOfService;
        }

        [Flags]
        private enum LSA_AccessPolicy : long
        {
            POLICY_VIEW_LOCAL_INFORMATION = 0x00000001L,
            POLICY_VIEW_AUDIT_INFORMATION = 0x00000002L,
            POLICY_GET_PRIVATE_INFORMATION = 0x00000004L,
            POLICY_TRUST_ADMIN = 0x00000008L,
            POLICY_CREATE_ACCOUNT = 0x00000010L,
            POLICY_CREATE_SECRET = 0x00000020L,
            POLICY_CREATE_PRIVILEGE = 0x00000040L,
            POLICY_SET_DEFAULT_QUOTA_LIMITS = 0x00000080L,
            POLICY_SET_AUDIT_REQUIREMENTS = 0x00000100L,
            POLICY_AUDIT_LOG_ADMIN = 0x00000200L,
            POLICY_SERVER_ADMIN = 0x00000400L,
            POLICY_LOOKUP_NAMES = 0x00000800L,
            POLICY_NOTIFICATION = 0x00001000L
        }

        //POLICY_ALL_ACCESS mask <http://msdn.microsoft.com/en-us/library/windows/desktop/ms721916%28v=vs.85%29.aspx>
        private const int POLICY_ALL_ACCESS = (int)(
            LSA_AccessPolicy.POLICY_AUDIT_LOG_ADMIN |
            LSA_AccessPolicy.POLICY_CREATE_ACCOUNT |
            LSA_AccessPolicy.POLICY_CREATE_PRIVILEGE |
            LSA_AccessPolicy.POLICY_CREATE_SECRET |
            LSA_AccessPolicy.POLICY_GET_PRIVATE_INFORMATION |
            LSA_AccessPolicy.POLICY_LOOKUP_NAMES |
            LSA_AccessPolicy.POLICY_NOTIFICATION |
            LSA_AccessPolicy.POLICY_SERVER_ADMIN |
            LSA_AccessPolicy.POLICY_SET_AUDIT_REQUIREMENTS |
            LSA_AccessPolicy.POLICY_SET_DEFAULT_QUOTA_LIMITS |
            LSA_AccessPolicy.POLICY_TRUST_ADMIN |
            LSA_AccessPolicy.POLICY_VIEW_AUDIT_INFORMATION |
            LSA_AccessPolicy.POLICY_VIEW_LOCAL_INFORMATION
        );

        [DllImport("advapi32.dll")]
        private static extern long LsaClose(IntPtr objectHandle);

        [DllImport("advapi32.dll")]
        private static extern long LsaNtStatusToWinError(long status);

        [DllImport("kernel32.dll", SetLastError=true, CharSet=CharSet.Auto)]
        private static extern bool GetComputerNameEx(COMPUTER_NAME_FORMAT NameType,
                           [Out] StringBuilder lpBuffer, ref uint lpnSize);

        [DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Auto)]
        public static extern bool ChangeServiceConfig(SafeHandle hService, uint dwServiceType,
        int dwStartType, int dwErrorControl, string lpBinaryPathName, string lpLoadOrderGroup,
        string lpdwTagId, string lpDependencies, string lpServiceStartName, string lpPassword,
        string lpDisplayName);

        #endregion
        #region Public interface

        public bool IsServiceAccount(SecurityIdentifier securityIdentifier)
        {
            NetIsServiceAccount(null, securityIdentifier.Translate(typeof(NTAccount)).Value, out var isServiceAccount);
            isServiceAccount |= securityIdentifier.IsWellKnown(WellKnownSidType.LocalSystemSid) ||
                                securityIdentifier.IsWellKnown(WellKnownSidType.LocalServiceSid) ||
                                securityIdentifier.IsWellKnown(WellKnownSidType.NetworkServiceSid);
            return isServiceAccount;
        }

        public void AddToGroup(SecurityIdentifier securityIdentifier, WellKnownSidType groupSid)
        {
            AddToGroup(securityIdentifier, new SecurityIdentifier(groupSid, null));
        }

        public void AddToGroup(SecurityIdentifier securityIdentifier, SecurityIdentifier groupIdentifier)
        {
            var groupSid = new byte[groupIdentifier.BinaryLength];
            groupIdentifier.GetBinaryForm(groupSid, 0);
            uint cchName = 0;
            var name = new StringBuilder();
            uint cchReferencedDomainName = 0;
            var referencedDomainName = new StringBuilder();
            ReturnCodes err = ReturnCodes.ERROR_NONE_MAPPED;
            if (!LookupAccountSid(null, groupSid, name, ref cchName, referencedDomainName, ref cchReferencedDomainName, out _))
            {
                err = (ReturnCodes)Marshal.GetLastWin32Error();
                if (err == ReturnCodes.ERROR_INSUFFICIENT_BUFFER)
                {
                    name.EnsureCapacity((int)cchName);
                    referencedDomainName.EnsureCapacity((int)cchReferencedDomainName);
                    err = ReturnCodes.NO_ERROR;
                    if (!LookupAccountSid(null, groupSid, name, ref cchName, referencedDomainName, ref cchReferencedDomainName, out _))
                    {
                        err = (ReturnCodes)Marshal.GetLastWin32Error();
                    }
                }
            }
            if (err == ReturnCodes.NO_ERROR)
            {
                AddToGroup(securityIdentifier, name.ToString());
                return;
            }

            throw new Exception($"Could not add user to group, failure to lookup group name: {err}");
        }

        public void AddToGroup(SecurityIdentifier securityIdentifier, string groupName)
        {
            ReturnCodes err;
            var sid = new byte[securityIdentifier.BinaryLength];
            securityIdentifier.GetBinaryForm(sid, 0);
            var info = new LOCALGROUP_MEMBERS_INFO_0
            {
                pSID = Marshal.AllocHGlobal(sid.Length)
            };

            try
            {
                Marshal.Copy(sid, 0, info.pSID, sid.Length);

                err = (ReturnCodes)NetLocalGroupAddMembers(null, groupName, 0, ref info, 1);
                if (err == ReturnCodes.NO_ERROR || err == ReturnCodes.ERROR_MEMBER_IN_ALIAS)
                {
                    return;
                }
            }
            finally
            {
                Marshal.FreeHGlobal(info.pSID);
            }
            throw new Exception($"Could not add user to group {groupName}: {err}");
        }

        public void AddPrivilege(SecurityIdentifier securityIdentifier, AccountRightsConstants accountRights)
        {
            var privilegeName = accountRights.ToString();
            var sid = new byte[securityIdentifier.BinaryLength];
            securityIdentifier.GetBinaryForm(sid, 0);
            var userRights = new[]
            {
                new LSA_UNICODE_STRING
                {
                    Buffer = Marshal.StringToHGlobalUni(privilegeName),
                    Length = (ushort)(privilegeName.Length * UnicodeEncoding.CharSize),
                    MaximumLength = (ushort)((privilegeName.Length + 1) * UnicodeEncoding.CharSize)
                }
            };
            var systemName = new LSA_UNICODE_STRING();
            var objectAttributes = new LSA_OBJECT_ATTRIBUTES();
            var status = LsaOpenPolicy(ref systemName, ref objectAttributes, POLICY_ALL_ACCESS, out var policyHandle);
            var winErrorCode = LsaNtStatusToWinError(status);
            if (winErrorCode != 0)
            {
                throw new Exception("LsaOpenPolicy failed", new Win32Exception((int)winErrorCode));
            }

            var pSid = Marshal.AllocHGlobal(sid.Length);
            try
            {

                Marshal.Copy(sid, 0, pSid, sid.Length);
                status = LsaAddAccountRights(policyHandle, pSid, userRights, userRights.Length);
                winErrorCode = LsaNtStatusToWinError(status);
                if (winErrorCode != 0)
                {
                    throw new Exception("LsaAddAccountRights failed", new Win32Exception((int)winErrorCode));
                }
            }
            finally
            {
                Marshal.FreeHGlobal(pSid);
                Marshal.FreeHGlobal(userRights[0].Buffer);
                LsaClose(policyHandle);
            }
        }

        public void SetUserPassword(string accountName, string password)
        {
            var userInfo = new USER_INFO_1003
            {
                sPassword = password
            };
            // A zero return indicates success.
            var result = NetUserSetInfo(null, accountName, 1003, ref userInfo, out _);
            if (result != 0)
            {
                throw new Win32Exception(result, $"Error while setting the password for {accountName}");
            }
        }

        public bool IsDomainController()
        {
            var serverInfo = NetServerGetInfo<SERVER_INFO_101>();
            if ((serverInfo.Type & ServerTypes.DomainCtrl) == ServerTypes.DomainCtrl
                || (serverInfo.Type & ServerTypes.BackupDomainCtrl) == ServerTypes.BackupDomainCtrl)
            {
                // Computer is a DC
                return true;
            }
            return false;
        }

        public string GetComputerDomain()
        {
            // Computer is a DC, default to domain name
            return Domain.GetComputerDomain().Name;
        }


        /// <summary>
        /// Checks whether or not a user account belongs to a domain or is a local account.
        /// </summary>
        /// <param name="userSid">The SID of the user.</param>
        /// <returns>True if the <paramref name="userSid"/> belongs to a domain, false otherwise.</returns>
        /// <exception cref="Exception">
        /// This method throws an exception if the second call
        /// to <see cref="GetWindowsAccountDomainSid"/> fails.</exception>
        public bool IsDomainAccount(SecurityIdentifier userSid)
        {
            var userBinSid = new byte[userSid.BinaryLength];
            userSid.GetBinaryForm(userBinSid, 0);
            uint sz = 0;
            if (!GetWindowsAccountDomainSid(userBinSid, null, ref sz) && sz == 0)
            {
                // This will fail if the SID is not a domain SID (i.e. a local account)
                return false;
            }
            var domainBinSid = new byte[sz];
            if (GetWindowsAccountDomainSid(userBinSid, domainBinSid, ref sz))
            {
                var domainSid = new SecurityIdentifier(domainBinSid, 0);
                var machineFound = LookupAccountName(
                    Environment.MachineName,
                    out _,
                    out _,
                    out var machineSid,
                    out _);
                if (machineFound)
                {
                    // If the machineSid is different from the domainSid
                    // that means it's a domain account.
                    return machineSid != domainSid;
                }
            }
            else
            {
                // That call should not fail
                throw new Exception("Unexpected failure while checking if the account belonged to a domain.");
            }

            return false;
        }

        public bool LookupAccountName(
            string accountName,
            out string user,
            out string domain,
            out SecurityIdentifier securityIdentifier,
            out SID_NAME_USE sidNameUse)
        {
            user = null;
            domain = null;
            byte[] sid = null;
            securityIdentifier = null;
            uint cbSid = 0;
            uint cchName = 0;
            var name = new StringBuilder();
            uint cchReferencedDomainName = 0;
            var referencedDomainName = new StringBuilder();
            ReturnCodes err;
            if (!LookupAccountName(null, accountName, null, ref cbSid, referencedDomainName, ref cchReferencedDomainName, out sidNameUse))
            {
                err = (ReturnCodes)Marshal.GetLastWin32Error();
                if (err == ReturnCodes.ERROR_INSUFFICIENT_BUFFER || err == ReturnCodes.ERROR_INVALID_FLAGS)
                {
                    sid = new byte[cbSid];
                    err = ReturnCodes.NO_ERROR;
                    if (!LookupAccountName(null, accountName, sid, ref cbSid, referencedDomainName, ref cchReferencedDomainName, out sidNameUse))
                    {
                        err = (ReturnCodes)Marshal.GetLastWin32Error();
                    }
                    else
                    {
                        securityIdentifier = new SecurityIdentifier(sid, 0);
                    }
                }
            }
            else
            {
                throw new Exception("could not call LookupAccountName");
            }

            if (err == ReturnCodes.NO_ERROR)
            {
                if (!LookupAccountSid(null, sid, name, ref cchName, referencedDomainName, ref cchReferencedDomainName, out sidNameUse))
                {
                    err = (ReturnCodes)Marshal.GetLastWin32Error();
                    if (err == ReturnCodes.ERROR_INSUFFICIENT_BUFFER)
                    {
                        name.EnsureCapacity((int)cchName);
                        referencedDomainName.EnsureCapacity((int)cchReferencedDomainName);
                        err = ReturnCodes.NO_ERROR;
                        if (!LookupAccountSid(null, sid, name, ref cchName, referencedDomainName, ref cchReferencedDomainName, out sidNameUse))
                        {
                            err = (ReturnCodes)Marshal.GetLastWin32Error();
                        }
                    }
                }
            }

            if (err == ReturnCodes.NO_ERROR)
            {
                domain = referencedDomainName.ToString();
                user = name.ToString();
                return true;
            }

            if (err == ReturnCodes.ERROR_NONE_MAPPED)
            {
                return false;
            }

            throw new Exception("unexpected error while looking account name");
        }

        public bool GetComputerName(COMPUTER_NAME_FORMAT format, out string name)
        {
            var nameBuilder = new StringBuilder();
            uint nSize = 260;
            nameBuilder.EnsureCapacity((int)nSize);
            var result = GetComputerNameEx(format, nameBuilder, ref nSize);
            name = nameBuilder.ToString();
            return result;
        }

        #endregion
    }
}
