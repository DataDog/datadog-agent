using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;
using System.IO;
using System;
using Datadog.CustomActions.Extensions;

namespace Datadog.CustomActions;

public class RestoreDaclRollbackCustomAction
{
    private readonly ISession _session;

    public RestoreDaclRollbackCustomAction(
        ISession session)
    {
        _session = session;
    }

    public ActionResult DoRollback()
    {
        try
        {
            var projectLocation = _session.Property("PROJECTLOCATION");
            _session.Log($"Resetting inheritance flag on \"{projectLocation}\"");
            if (!Directory.Exists(projectLocation))
            {
                _session.Log($"Directory {projectLocation} does not exists !");
                return ActionResult.Success;
            }
            // Restore the inheritance flag that is removed by MSI's usage of
            // obsolete SetFileSecurity API
            var dInfo = new DirectoryInfo(projectLocation);
            var dSecurity = dInfo.GetAccessControl();
            // Second param is ignored because of the first one
            // https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.objectsecurity.setaccessruleprotection?view=netframework-4.6.2
            dSecurity.SetAccessRuleProtection(false, true);
            dInfo.SetAccessControl(dSecurity);
        }
        catch (Exception e)
        {
            _session.Log($"Error while setting ACE: {e}");
            // Don't fail in cleanup/rollback actions otherwise
            // we may brick the installation.
        }

        return ActionResult.Success;
    }

    [CustomAction]
    public static ActionResult DoRollback(Session session)
    {
        return new RestoreDaclRollbackCustomAction(new SessionWrapper(session)).DoRollback();
    }
}
