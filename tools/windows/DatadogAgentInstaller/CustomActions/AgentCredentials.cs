using System.Security.Principal;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions
{
    // Shared account rules and access to the current credentials. No password backups.
    internal static class AgentCredentials
    {
        internal static string AccountSidKey => ConfigureUserCustomActions.AgentPasswordPrivateDataKey() + "_sid";

        internal static bool IsBuiltIn(SecurityIdentifier sid)
        {
            return sid.IsWellKnown(WellKnownSidType.LocalSystemSid)
                || sid.IsWellKnown(WellKnownSidType.LocalServiceSid)
                || sid.IsWellKnown(WellKnownSidType.NetworkServiceSid);
        }

        internal static bool RequiresPassword(INativeMethods native, SecurityIdentifier sid)
        {
            return !IsBuiltIn(sid) && !native.IsServiceAccount(sid);
        }

        // Without an expected SID, retain the existing installation's legacy-secret lookup.
        // Removal and rollback require an explicit association with the outgoing account.
        internal static string ReadPassword(INativeMethods native, SecurityIdentifier expectedSid = null)
        {
            if (expectedSid is not null && native.FetchSecret(AccountSidKey) != expectedSid.Value)
            {
                return null;
            }
            return native.FetchSecret(ConfigureUserCustomActions.AgentPasswordPrivateDataKey());
        }

        internal static void StorePassword(INativeMethods native, SecurityIdentifier sid, string password)
        {
            // Invalidate the association first: a failed write must never label another
            // account's password with the previous SID. Rollback does not undo these writes.
            native.StoreSecret(AccountSidKey, "unassociated");
            native.StoreSecret(ConfigureUserCustomActions.AgentPasswordPrivateDataKey(), password);
            native.StoreSecret(AccountSidKey, sid.Value);
        }

        internal static void RemovePassword(INativeMethods native)
        {
            native.StoreSecret(AccountSidKey, "unassociated");
            native.RemoveSecret(ConfigureUserCustomActions.AgentPasswordPrivateDataKey());
            native.RemoveSecret(AccountSidKey);
        }

        internal static void NormalizeServiceCredentials(bool isServiceAccount, SecurityIdentifier sid,
            ref string account, ref string password)
        {
            if (sid.IsWellKnown(WellKnownSidType.LocalSystemSid))
            {
                account = "LocalSystem";
            }
            else if (sid.IsWellKnown(WellKnownSidType.LocalServiceSid))
            {
                account = "LocalService";
            }
            else if (sid.IsWellKnown(WellKnownSidType.NetworkServiceSid))
            {
                account = "NetworkService";
            }

            if (!isServiceAccount && string.IsNullOrEmpty(password))
            {
                password = null;
            }
            else if (isServiceAccount)
            {
                password = IsBuiltIn(sid) ? "" : null;
            }
        }
    }
}
