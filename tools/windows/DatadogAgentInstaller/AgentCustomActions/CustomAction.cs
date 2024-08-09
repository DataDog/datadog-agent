using Datadog.CustomActions;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.AgentCustomActions
{
    /// <summary>
    /// Defines the custom action entry points for the Datadog Agent MSI.
    /// </summary>
    /// <remarks>
    /// See AgentCustomActions.cs in the WixSetup project for the WiX/MSI table configuration for these custom actions.
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
        public static ActionResult ReadConfig(Session session)
        {
            return Datadog.CustomActions.ConfigCustomActions.ReadConfig(session);
        }

        [CustomAction]
        public static ActionResult WriteConfig(Session session)
        {
            return Datadog.CustomActions.ConfigCustomActions.WriteConfig(session);
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
        public static ActionResult Patch(Session session)
        {
            return Datadog.CustomActions.PatchInstallerCustomAction.Patch(session);
        }

        [CustomAction]
        public static ActionResult ReportFailure(Session session)
        {
            return Datadog.CustomActions.Telemetry.ReportFailure(session);
        }

        [CustomAction]
        public static ActionResult ReportSuccess(Session session)
        {
            return Datadog.CustomActions.Telemetry.ReportSuccess(session);
        }

        [CustomAction]
        public static ActionResult EnsureNpmServiceDependency(Session session)
        {
            return Datadog.CustomActions.ServiceCustomAction.EnsureNpmServiceDependency(session);
        }

        [CustomAction]
        public static ActionResult CleanupFiles(Session session)
        {
            return Datadog.CustomActions.CleanUpFilesCustomAction.CleanupFiles(session);
        }

        [CustomAction]
        public static ActionResult DecompressPythonDistributions(Session session)
        {
            return Datadog.CustomActions.PythonDistributionCustomAction.DecompressPythonDistributions(session);
        }

        [CustomAction]
        public static ActionResult PrepareDecompressPythonDistributions(Session session)
        {
            return Datadog.CustomActions.PythonDistributionCustomAction.PrepareDecompressPythonDistributions(session);
        }

        [CustomAction]
        public static ActionResult ConfigureUser(Session session)
        {
            return Datadog.CustomActions.ConfigureUserCustomActions.ConfigureUser(session);
        }

        [CustomAction]
        public static ActionResult ConfigureUserRollback(Session session)
        {
            return Datadog.CustomActions.ConfigureUserCustomActions.ConfigureUserRollback(session);
        }

        [CustomAction]
        public static ActionResult UninstallUser(Session session)
        {
            return Datadog.CustomActions.ConfigureUserCustomActions.UninstallUser(session);
        }

        [CustomAction]
        public static ActionResult UninstallUserRollback(Session session)
        {
            return Datadog.CustomActions.ConfigureUserCustomActions.UninstallUserRollback(session);
        }

        [CustomAction]
        public static ActionResult ProcessDdAgentUserCredentials(Session session)
        {
            return Datadog.CustomActions.ProcessUserCustomActions.ProcessDdAgentUserCredentials(session);
        }

        [CustomAction]
        public static ActionResult ProcessDdAgentUserCredentialsUI(Session session)
        {
            return Datadog.CustomActions.ProcessUserCustomActions.ProcessDdAgentUserCredentialsUI(session);
        }

        [CustomAction]
        public static ActionResult SendFlare(Session session)
        {
            return Datadog.CustomActions.Flare.SendFlare(session);
        }

        [CustomAction]
        public static ActionResult ConfigureServices(Session session)
        {
            return Datadog.CustomActions.ServiceCustomAction.ConfigureServices(session);
        }

        [CustomAction]
        public static ActionResult ConfigureServicesRollback(Session session)
        {
            return Datadog.CustomActions.ServiceCustomAction.ConfigureServicesRollback(session);
        }

        [CustomAction]
        public static ActionResult StopDDServices(Session session)
        {
            return Datadog.CustomActions.ServiceCustomAction.StopDDServices(session);
        }

        [CustomAction]
        public static ActionResult StartDDServices(Session session)
        {
            return Datadog.CustomActions.ServiceCustomAction.StartDDServices(session);
        }

        [CustomAction]
        public static ActionResult StartDDServicesRollback(Session session)
        {
            return Datadog.CustomActions.ServiceCustomAction.StartDDServicesRollback(session);
        }

        [CustomAction]
        public static ActionResult UninstallWriteInstallState(Session session)
        {
            return new WriteInstallStateCA(new SessionWrapper(session)).UninstallWriteInstallState();
        }

        [CustomAction]
        public static ActionResult DDCreateFolders(Session session)
        {
            return Datadog.CustomActions.ConfigCustomActions.DDCreateFolders(session);
        }

        [CustomAction]
        public static ActionResult DoRollback(Session session)
        {
            return Datadog.CustomActions.Rollback.RestoreDaclRollbackCustomAction.DoRollback(session);
        }

        [CustomAction]
        public static ActionResult WriteInstallInfo(Session session)
        {
            return Datadog.CustomActions.InstallInfoCustomActions.WriteInstallInfo(session);
        }
    }
}
