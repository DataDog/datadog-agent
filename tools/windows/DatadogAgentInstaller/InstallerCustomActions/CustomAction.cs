using Datadog.CustomActions;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.InstallerCustomActions
{

    /// <summary>
    /// Defines the custom action entry points for the Datadog Installer MSI.
    /// </summary>
    /// <remarks>
    /// See DatadogInstallerCustomActions.cs in the WixSetup project for the WiX/MSI table configuration for these custom actions.
    /// </remarks>
    public class CustomActions
    {
        [CustomAction]
        public static ActionResult EnsureAdminCaller(Session session)
        {
            return Datadog.CustomActions.PrerequisitesCustomActions.EnsureAdminCaller(session);
        }

        [CustomAction]
        public static ActionResult OpenMsiLog(Session session)
        {
            return Datadog.CustomActions.MsiLogCustomActions.OpenMsiLog(session);
        }

        [CustomAction]
        public static ActionResult ReadInstallState(Session session)
        {
            return new ReadInstallStateCA(new SessionWrapper(session)).ReadInstallState();
        }

        [CustomAction]
        public static ActionResult WriteInstallState(Session session)
        {
            return new WriteInstallStateCA(new SessionWrapper(session)).WriteInstallState();
        }

        [CustomAction]
        public static ActionResult DeleteInstallState(Session session)
        {
            return new WriteInstallStateCA(new SessionWrapper(session)).DeleteInstallState();
        }

        [CustomAction]
        public static ActionResult ReadConfig(Session session)
        {
            return Datadog.CustomActions.ConfigCustomActions.ReadConfig(session);
        }

        [CustomAction]
        public static ActionResult ProcessDdAgentUserCredentials(Session session)
        {
            return Datadog.CustomActions.ProcessUserCustomActions.ProcessDdAgentUserCredentials(session);
        }
    }
}
