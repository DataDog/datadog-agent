using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.IO;
using System.Text;
using System.Xml.Linq;

namespace Datadog.CustomActions
{
    public class AiUsageDesktopMonitorCustomAction
    {
        private const string TaskName = "Datadog AI Usage Agent";
        private const string TaskDescription = "Starts the Datadog AI Usage Agent desktop monitor in the interactive user session.";
        private const string UsersGroupSid = "S-1-5-32-545";

        private static ActionResult Configure(ISession session)
        {
            var projectLocation = session.Property("PROJECTLOCATION");
            var configRoot = session.Property("APPLICATIONDATADIRECTORY");
            if (string.IsNullOrEmpty(projectLocation) || string.IsNullOrEmpty(configRoot))
            {
                session.Log("Skipping AI Usage Agent desktop monitor task registration: install paths are unavailable.");
                return ActionResult.Success;
            }

            var hostPath = Path.Combine(projectLocation, "bin", "agent", "ai-usage-agent-native-host.exe");
            var configPath = Path.Combine(configRoot, "ai_usage_native_host.yaml");
            var taskXmlPath = Path.Combine(Path.GetTempPath(), $"datadog-ai-usage-agent-{Guid.NewGuid():N}.xml");
            var schtasks = Path.Combine(Environment.SystemDirectory, "schtasks.exe");

            try
            {
                File.WriteAllText(taskXmlPath, BuildTaskXml(hostPath, configPath), Encoding.Unicode);
                using (var proc = session.RunCommand(
                           schtasks,
                           $"/Create /TN \"{TaskName}\" /XML \"{taskXmlPath}\" /F"))
                {
                    if (proc.ExitCode != 0)
                    {
                        session.Log($"AI Usage Agent desktop monitor task registration exited with code: {proc.ExitCode}");
                        return ActionResult.Failure;
                    }
                }

                session.Log("AI Usage Agent desktop monitor task registered.");
                using (var runProc = session.RunCommand(schtasks, $"/Run /TN \"{TaskName}\""))
                {
                    if (runProc.ExitCode != 0)
                    {
                        session.Log($"AI Usage Agent desktop monitor task start exited with code: {runProc.ExitCode}");
                    }
                }

                return ActionResult.Success;
            }
            finally
            {
                try
                {
                    File.Delete(taskXmlPath);
                }
                catch (Exception e)
                {
                    session.Log($"Failed to delete temporary AI Usage Agent task XML: {e}");
                }
            }
        }

        private static ActionResult Remove(ISession session)
        {
            var schtasks = Path.Combine(Environment.SystemDirectory, "schtasks.exe");
            using (var endProc = session.RunCommand(schtasks, $"/End /TN \"{TaskName}\""))
            {
                if (endProc.ExitCode != 0)
                {
                    session.Log($"AI Usage Agent desktop monitor task end exited with code: {endProc.ExitCode}");
                }
            }

            using (var deleteProc = session.RunCommand(schtasks, $"/Delete /TN \"{TaskName}\" /F"))
            {
                if (deleteProc.ExitCode != 0)
                {
                    session.Log($"AI Usage Agent desktop monitor task deletion exited with code: {deleteProc.ExitCode}");
                }
            }

            return ActionResult.Success;
        }

        private static string BuildTaskXml(string hostPath, string configPath)
        {
            XNamespace ns = "http://schemas.microsoft.com/windows/2004/02/mit/task";
            var document = new XDocument(
                new XDeclaration("1.0", "UTF-16", null),
                new XElement(ns + "Task",
                    new XAttribute("version", "1.4"),
                    new XElement(ns + "RegistrationInfo",
                        new XElement(ns + "Author", "Datadog"),
                        new XElement(ns + "Description", TaskDescription)),
                    new XElement(ns + "Triggers",
                        new XElement(ns + "LogonTrigger",
                            new XElement(ns + "Enabled", "true"))),
                    new XElement(ns + "Principals",
                        new XElement(ns + "Principal",
                            new XAttribute("id", "Author"),
                            new XElement(ns + "GroupId", UsersGroupSid),
                            new XElement(ns + "RunLevel", "LeastPrivilege"))),
                    new XElement(ns + "Settings",
                        new XElement(ns + "MultipleInstancesPolicy", "Parallel"),
                        new XElement(ns + "DisallowStartIfOnBatteries", "false"),
                        new XElement(ns + "StopIfGoingOnBatteries", "false"),
                        new XElement(ns + "AllowHardTerminate", "true"),
                        new XElement(ns + "StartWhenAvailable", "false"),
                        new XElement(ns + "RunOnlyIfNetworkAvailable", "false"),
                        new XElement(ns + "AllowStartOnDemand", "true"),
                        new XElement(ns + "Enabled", "true"),
                        new XElement(ns + "Hidden", "false"),
                        new XElement(ns + "RunOnlyIfIdle", "false"),
                        new XElement(ns + "WakeToRun", "false"),
                        new XElement(ns + "RestartOnFailure",
                            new XElement(ns + "Interval", "PT1M"),
                            new XElement(ns + "Count", "3")),
                        new XElement(ns + "ExecutionTimeLimit", "PT0S"),
                        new XElement(ns + "Priority", "7")),
                    new XElement(ns + "Actions",
                        new XAttribute("Context", "Author"),
                        new XElement(ns + "Exec",
                            new XElement(ns + "Command", hostPath),
                            new XElement(ns + "Arguments", $"--desktop-monitor --config \"{configPath}\"")))));

            return document.ToString(SaveOptions.DisableFormatting);
        }

        public static ActionResult Configure(Session session)
        {
            return Configure(new SessionWrapper(session));
        }

        public static ActionResult Remove(Session session)
        {
            return Remove(new SessionWrapper(session));
        }
    }
}
