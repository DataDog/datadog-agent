using Datadog.CustomActions.Extensions;
using Microsoft.Deployment.WindowsInstaller;
using System.Collections.Generic;
using System.IO;
using System;

namespace Datadog.CustomActions
{
    public class CleanUpFilesCustomAction
    {
        private static void RemovePythonDistributions(string projectLocation, ISession session)
        {
            var embeddedFolders = new List<string>
            {
                Path.Combine(projectLocation, "embedded2"),
                Path.Combine(projectLocation, "embedded3")
            };
            try
            {
                foreach (var embeddedDist in embeddedFolders)
                {
                    if (Directory.Exists(embeddedDist))
                    {
                        session.Log($"{embeddedDist} found, deleting.");
                        Directory.Delete(embeddedDist, true);
                    }
                    else
                    {
                        session.Log($"{embeddedDist} not found, skip deletion.");
                    }
                }
            }
            catch (Exception e)
            {
                session.Log($"Error while deleting embedded distribution: {e}");
            }
        }

        private static ActionResult CleanupFiles(ISession session)
        {
            var projectLocation = session.Property("PROJECTLOCATION");
            var applicationDataLocation = session.Property("APPLICATIONDATADIRECTORY");
            RemovePythonDistributions(projectLocation, session);
            try
            {
                var authToken = Path.Combine(applicationDataLocation, "auth_token");

                if (File.Exists(authToken))
                {
                    session.Log($"{authToken} found, deleting.");
                    File.Delete(authToken);
                }
                else
                {
                    session.Log($"{authToken} not found exists, skip deletion.");
                }
            }
            catch (Exception e)
            {
                session.Log($"Error while deleting file: {e}");
                return ActionResult.Failure;
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
