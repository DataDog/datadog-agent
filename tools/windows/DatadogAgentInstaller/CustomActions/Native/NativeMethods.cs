using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Text;

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

    public static class NativeMethods
    {
        public enum ReturnCodes
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

        [DllImport("credui.dll", EntryPoint = "CredUIParseUserNameW", CharSet = CharSet.Unicode)]
        private static extern ReturnCodes CredUIParseUserName(
            string userName,
            StringBuilder user,
            int userMaxChars,
            StringBuilder domain,
            int domainMaxChars);

        /// <summary>
        /// Extracts the domain and user account name from a fully qualified user name.
        /// </summary>
        /// <param name="userName">A <see cref="string"/> that contains the user name to be parsed. The name must be in UPN or down-level format, or a certificate.</param>
        /// <param name="user">A <see cref="string"/> that receives the user account name.</param>
        /// <param name="domain">A <see cref="string"/> that receives the domain name. If <paramref name="userName"/> specifies a certificate, pszDomain will be <see langword="null"/>.</param>
        /// <returns>
        ///     <see langword="true"/> if the <paramref name="userName"/> contains a domain and a user-name; otherwise, <see langword="false"/>.
        /// </returns>
        public static bool ParseUserName(string userName, out string user, out string domain)
        {
            if (string.IsNullOrEmpty(userName))
            {
                throw new ArgumentNullException(nameof(userName));
            }

            StringBuilder userBuilder = new StringBuilder();
            StringBuilder domainBuilder = new StringBuilder();

            ReturnCodes returnCode = CredUIParseUserName(userName, userBuilder, int.MaxValue, domainBuilder, int.MaxValue);
            switch (returnCode)
            {
                case ReturnCodes.NO_ERROR: // The username is valid.
                    user = userBuilder.ToString();
                    domain = domainBuilder.ToString();
                    return true;

                case ReturnCodes.ERROR_INVALID_ACCOUNT_NAME: // The username is not valid.
                    user = userName;
                    domain = null;
                    return false;

                // Impossible to reach this state
                //case ReturnCodes.ERROR_INSUFFICIENT_BUFFER: // One of the buffers is too small.
                //    throw new OutOfMemoryException();

                case ReturnCodes.ERROR_INVALID_PARAMETER: // ulUserMaxChars or ulDomainMaxChars is zero OR userName, user, or domain is NULL.
                    throw new ArgumentNullException("userName");

                default:
                    user = null;
                    domain = null;
                    return false;
            }
        }

        public enum NtStatus : uint
        {
            Success = 0x00000000
        }

        [DllImport("logoncli.dll", EntryPoint = "NetIsServiceAccount", CharSet = CharSet.Unicode)]
        public static extern NtStatus NetIsServiceAccount(
            string serverName,
            string accountName,
            out bool isService);

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

        public static bool LookupAccountName(string accountName, out string user, out string domain, out SecurityIdentifier securityIdentifier, out SID_NAME_USE sidNameUse)
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

        public static void AddToGroup(this SecurityIdentifier securityIdentifier, WellKnownSidType groupSid)
        {
            AddToGroup(securityIdentifier, new SecurityIdentifier(groupSid, null));
        }

        public static void AddToGroup(this SecurityIdentifier securityIdentifier, SecurityIdentifier groupIdentifier)
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

        public static void AddToGroup(this SecurityIdentifier securityIdentifier, string groupName)
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

        [Flags]
        public enum ServerTypes : uint
        {
            DomainCtrl= 0x00000008,
            BackupDomainCtrl= 0x00000010,
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

        #region Add/Remove privileges
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

        public static void AddPrivilege(this SecurityIdentifier securityIdentifier, AccountRightsConstants accountRights)
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
        #endregion
    }
}
