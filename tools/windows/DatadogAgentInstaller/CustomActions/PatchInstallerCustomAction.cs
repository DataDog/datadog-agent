using System;
using Microsoft.Deployment.WindowsInstaller;
using Microsoft.Win32;

namespace Datadog.CustomActions;

public class PatchInstallerCustomAction
{
    private static void RenameSubKey(string path, string keyName, string newKeyName)
    {
        if (Environment.Is64BitOperatingSystem && !Environment.Is64BitProcess)
        {
            var rk64 = RegistryKey.OpenBaseKey(RegistryHive.LocalMachine, RegistryView.Registry64).OpenSubKey(path, RegistryKeyPermissionCheck.ReadWriteSubTree);

            var objValue = rk64.GetValue(keyName);
            var valKind = rk64.GetValueKind(keyName);

            rk64.SetValue(newKeyName, objValue, valKind);
            rk64.DeleteValue(keyName);
        }

        using var rk = Registry.LocalMachine.OpenSubKey(path, RegistryKeyPermissionCheck.ReadWriteSubTree);
        {
            var objValue = rk.GetValue(keyName);
            var valKind = rk.GetValueKind(keyName);

            rk.SetValue(newKeyName, objValue, valKind);
            rk.DeleteValue(keyName);
        }
    }

    /// <summary>
    /// Patch the previous install to make the upgrade work.
    /// </summary>
    /// <param name="s">The session object.</param>
    /// <returns><see cref="ActionResult.Success"/></returns>
    [CustomAction]
    public static ActionResult Patch(Session s)
    {
        ISession session = new SessionWrapper(s);
        try
        {
            // Prevent the previous installer from deleting the services on rollback
            RenameSubKey(@"Software\Datadog\Datadog Agent\installRollback\", "Installed Services", "_Installed Services");

            // Prevent the previous installer from deleting the ddagentuser on rollback
            RenameSubKey(@"Software\Datadog\Datadog Agent\installRollback\", "CreatedDDUser", "_CreatedDDUser"); 
        }
        catch (Exception e)
        {
            // Don't need full stack trace
            session.Log($"Cannot patch previous installation: {e.Message}");
        }

        return ActionResult.Success;
    }

    /// <summary>
    /// Undo the patch done in the <see cref="Patch"/> function.
    /// </summary>
    /// Ideally we would schedule this as a rollback for the `<see cref="Patch"/> function
    /// and it would run as the last rollback, unfortunately the previous installer
    /// rollback runs after this one no matter which order we put the custom action in.
    /// <param name="s">The session object.</param>
    /// <returns><see cref="ActionResult.Success"/></returns>
    public static ActionResult Unpatch(Session s)
    {
        ISession session = new SessionWrapper(s);
        try
        {
            RenameSubKey(@"Software\Datadog\Datadog Agent\installRollback", "_Installed Services", "Installed Services");
            RenameSubKey(@"Software\Datadog\Datadog Agent\installRollback", "_CreatedDDUser", "CreatedDDUser");
        }
        catch (Exception e)
        {
            // Don't need full stack trace
            session.Log($"Cannot unpatch previous installation: {e.Message}");
        }

        return ActionResult.Success;
    }
}
