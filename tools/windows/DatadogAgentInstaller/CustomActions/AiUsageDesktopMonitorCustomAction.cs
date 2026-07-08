using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Microsoft.Win32;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.IO;

namespace Datadog.CustomActions
{
    // Legacy cleanup for the AI Usage native host. Pre-migration Agents installed the AI Usage
    // Chrome native messaging host directly from the MSI (binary, registry entries, generated
    // config/manifest, and a "Datadog AI Usage Agent" scheduled task). The host is now delivered
    // as the "ai-usage" fleet installer extension (gated on EUDM). This custom action removes the
    // artifacts that the MSI did not track (the scheduled task and the generated manifest) and the
    // Chrome registry entries, so upgrading from a pre-migration Agent does not leave them orphaned.
    public class AiUsageDesktopMonitorCustomAction
    {
        private const string TaskName = "Datadog AI Usage Agent";
        private const string NativeHostName = "com.datadoghq.ai_usage_agent.native_host";
        private const string ObsoleteNativeHostName = "com.datadoghq.ai_prompt_logger.native_host";

        private static readonly string[] ChromeRegistryKeys =
        {
            @"Software\Google\Chrome\NativeMessagingHosts\" + NativeHostName,
            @"Software\WOW6432Node\Google\Chrome\NativeMessagingHosts\" + NativeHostName,
        };

        private static ActionResult Remove(ISession session)
        {
            RemoveScheduledTask(session);
            RemoveChromeRegistryKeys(session);
            RemoveManifests(session);
            return ActionResult.Success;
        }

        private static void RemoveScheduledTask(ISession session)
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
        }

        private static void RemoveChromeRegistryKeys(ISession session)
        {
            try
            {
                using var hklm = RegistryKey.OpenBaseKey(RegistryHive.LocalMachine, RegistryView.Registry64);
                foreach (var keyPath in ChromeRegistryKeys)
                {
                    hklm.DeleteSubKeyTree(keyPath, throwOnMissingSubKey: false);
                }
            }
            catch (Exception e)
            {
                session.Log($"Failed to remove AI Usage Chrome registry keys: {e}");
            }
        }

        private static void RemoveManifests(ISession session)
        {
            var projectLocation = session.Property("PROJECTLOCATION");
            if (string.IsNullOrEmpty(projectLocation))
            {
                return;
            }

            var manifestDir = Path.Combine(projectLocation, "bin", "agent", "dist");
            foreach (var hostName in new[] { NativeHostName, ObsoleteNativeHostName })
            {
                var manifestPath = Path.Combine(manifestDir, $"{hostName}.json");
                try
                {
                    if (File.Exists(manifestPath))
                    {
                        File.Delete(manifestPath);
                    }
                }
                catch (Exception e)
                {
                    session.Log($"Failed to remove AI Usage native messaging manifest \"{manifestPath}\": {e}");
                }
            }
        }

        public static ActionResult Remove(Session session)
        {
            return Remove(new SessionWrapper(session));
        }
    }
}
