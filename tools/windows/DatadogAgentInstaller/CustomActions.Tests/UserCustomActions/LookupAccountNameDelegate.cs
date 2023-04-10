using System.Security.Principal;
using Datadog.CustomActions.Native;

namespace CustomActions.Tests.UserCustomActions
{
    // Return type must be void or MoQ won't accept it.
    delegate void LookupAccountNameDelegate(
        string accountName,
        out string user,
        out string domain,
        out SecurityIdentifier securityIdentifier,
        out SID_NAME_USE sidNameUse);
}
