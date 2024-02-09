using System;
using System.IO;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions.Rollback
{
    public class RestoreDaclRollbackCustomAction
    {
        private readonly ISession _session;

        public RestoreDaclRollbackCustomAction(
            ISession session)
        {
            _session = session;
        }

        public static void RestoreAutoInheritedFlag(string path)
        {
            if (!Directory.Exists(path))
            {
                throw new DirectoryNotFoundException(path);
            }
            // Restore the inheritance flag that is removed by MSI's usage of
            // obsolete SetFileSecurity API
            var dInfo = new DirectoryInfo(path);
            var dSecurity = dInfo.GetAccessControl();
            // Second param is ignored because of the first one
            // https://learn.microsoft.com/en-us/dotnet/api/system.security.accesscontrol.objectsecurity.setaccessruleprotection?view=netframework-4.6.2
            dSecurity.SetAccessRuleProtection(false, true);
            dInfo.SetAccessControl(dSecurity);
        }

        public ActionResult DoRollback()
        {
            try
            {
                var projectLocation = _session.Property("PROJECTLOCATION");
                _session.Log($"Resetting inheritance flag on \"{projectLocation}\"");
                RestoreAutoInheritedFlag(projectLocation);
            }
            catch (DirectoryNotFoundException dnfex)
            {
                // That's fine
                _session.Log($"Directory {dnfex.Message} does not exists !");
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
}
