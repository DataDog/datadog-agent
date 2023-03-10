using System;
using Datadog.CustomActions.Extensions;
using Microsoft.Deployment.WindowsInstaller;
using Microsoft.Win32;

namespace Datadog.CustomActions
{
    public class NpmCustomAction
    {
        private static ActionResult EnsureNpmServiceDepdendency(ISession session)
        {
            try
            {
                var addLocal = session.Property("ADDLOCAL").ToUpper();
                session.Log($"ADDLOCAL={addLocal}");
                using var systemProbeDef = Registry.LocalMachine.OpenSubKey(@"SYSTEM\CurrentControlSet\Services\datadog-system-probe", true);
                if (systemProbeDef != null)
                {
                    if (string.IsNullOrEmpty(addLocal))
                    {
                        systemProbeDef.SetValue("DependOnService", new[]
                        {
                            "datadogagent"
                        }, RegistryValueKind.MultiString);

                    }
                    else if (addLocal.Contains("NPM") ||
                             addLocal.Contains("ALL"))
                    {
                        systemProbeDef.SetValue("DependOnService", new[]
                        {
                            "datadogagent",
                            "ddnpm"
                        }, RegistryValueKind.MultiString);
                    }
                }
                else
                {
                    session.Log("Registry key does not exist");
                }
            }
            catch (Exception e)
            {
                session.Log($"Could not update system probe dependent service: {e.Message}");
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult EnsureNpmServiceDependency(Session session)
        {
            return EnsureNpmServiceDepdendency(new SessionWrapper(session));
        }
    }
}
