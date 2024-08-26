using System.DirectoryServices.ActiveDirectory;
using System.Security.Principal;
using Datadog.CustomActions.Native;

namespace Datadog.CustomActions.Interfaces
{
    public interface INativeMethods
    {
        bool IsServiceAccount(SecurityIdentifier securityIdentifier);

        void AddToGroup(SecurityIdentifier securityIdentifier, WellKnownSidType groupSid);

        void AddToGroup(SecurityIdentifier securityIdentifier, SecurityIdentifier groupIdentifier);

        void AddToGroup(SecurityIdentifier securityIdentifier, string groupName);

        void AddPrivilege(SecurityIdentifier securityIdentifier, AccountRightsConstants accountRights);

        bool LookupAccountName(
            string accountName,
            out string user,
            out string domain,
            out SecurityIdentifier securityIdentifier,
            out SID_NAME_USE sidNameUse);

        void SetUserPassword(string accountName, string password);

        bool IsDomainController();

        bool IsReadOnlyDomainController();

        /// <summary>
        /// 
        /// </summary>
        /// <exception cref="ActiveDirectoryObjectNotFoundException">
        /// Thrown if the computer is not part of a domain.
        /// </exception>
        /// <returns></returns>
        string GetComputerDomain();

        bool IsDomainAccount(SecurityIdentifier userSid);

        bool GetComputerName(COMPUTER_NAME_FORMAT format, out string name);
        int AddUser(string userName, string userPassword);

        void EnablePrivilege(string privilegeName);

        void GetCurrentUser(out string name, out SecurityIdentifier sid);

        string GetVersionString(string product);
    }
}
