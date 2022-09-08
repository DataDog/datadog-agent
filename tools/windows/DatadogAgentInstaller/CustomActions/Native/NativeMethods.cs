using System;
using System.Runtime.InteropServices;
using System.Text;

namespace CustomActions.Native
{
    public static class NativeMethods
    {
        public enum ReturnCodes
        {
            NO_ERROR = 0,
            ERROR_CANCELLED = 1223,
            ERROR_NO_SUCH_LOGON_SESSION = 1312,
            ERROR_NOT_FOUND = 1168,
            ERROR_INVALID_ACCOUNT_NAME = 1315,
            ERROR_INSUFFICIENT_BUFFER = 122,
            ERROR_INVALID_PARAMETER = 87,
            ERROR_INVALID_FLAGS = 1004,
            ERROR_BAD_ARGUMENTS = 160,
            ERROR_NONE_MAPPED = 1332
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
        [System.Diagnostics.CodeAnalysis.SuppressMessage("Microsoft.Design", "CA1021:AvoidOutParameters")]
        public static bool ParseUserName(string userName, out string user, out string domain)
        {
            if (string.IsNullOrEmpty(userName))
            {
                throw new ArgumentNullException("userName");
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

        public static bool LookupAccountName(string accountName, out string user, out string domain, out SID_NAME_USE sidNameUse)
        {
            user = null;
            domain = null;
            byte[] sid = null;
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
    }
}
