using Datadog.CustomActions.Extensions;
using Microsoft.Deployment.WindowsInstaller;
using System.IO;
using System;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions
{
    public class CleanUpFilesCustomAction
    {
        private static ActionResult CleanupFiles(ISession session)
        {
            var projectLocation = session.Property("PROJECTLOCATION");
            var applicationDataLocation = session.Property("APPLICATIONDATADIRECTORY");
            var toDelete = new[]
            {
                Path.Combine(projectLocation, "embedded2"),
                Path.Combine(projectLocation, "embedded3"),
                Path.Combine(applicationDataLocation, "install_info"),
                Path.Combine(applicationDataLocation, "auth_token"),
                Path.Combine(applicationDataLocation, "conf.d", "win32_event_log.d", "profiles", "dd_security_events_high.yaml"),
                Path.Combine(applicationDataLocation, "conf.d", "win32_event_log.d", "profiles", "dd_security_events_low.yaml"),
            };
            foreach (var path in toDelete)
            {
                try
                {
                    if (Directory.Exists(path))
                    {
                        session.Log($"Deleting directory \"{path}\"");
                        Directory.Delete(path, true);
                    }
                    else if (File.Exists(path))
                    {
                        session.Log($"Deleting file \"{path}\"");
                        File.Delete(path);
                    }
                    else
                    {
                        session.Log($"{path} not found, skip deletion.");
                    }
                }
                catch (Exception e)
                {
                    session.Log($"Error while deleting file: {e}");
                    // Don't fail in cleanup/rollback actions otherwise
                    // we may brick the installation.
                }
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult CleanupFiles(Session session)
        {
            return CleanupFiles(new SessionWrapper(session));
        }
    }
}
