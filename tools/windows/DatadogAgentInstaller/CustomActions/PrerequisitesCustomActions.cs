using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.Diagnostics;
using System.Security.Principal;

namespace Datadog.CustomActions
{
    public class PrerequisitesCustomActions
    {
        const string Error = "The Datadog Agent installer must be run by a user that is a member of the Administrator group.";

        /// <summary>
        /// Set by <see cref="EnsureSecureConfigRootUI"/> to tell the UI whether it can continue.
        /// </summary>
        public const string ConfigRootValidProperty = "DDConfigRoot_Valid";

        public static ActionResult EnsureAdminCaller(Session session)
        {
            if (!new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(WindowsBuiltInRole.Administrator))
            {
                ((ISession)new SessionWrapper(session)).Log(Error);
                if (int.Parse(session["UILevel"]) > 3)
                {
                    try
                    {
                        // Skip the fatal error dialog and run the installer again as an administrator
                        session["SKIP_ERROR_DIALOG"] = "1";

                        var startInfo = new ProcessStartInfo
                        {
                            UseShellExecute = true,
                            WorkingDirectory = Environment.CurrentDirectory,
                            FileName = "msiexec.exe",
                            Arguments = "/i \"" + session["OriginalDatabase"] + "\"",
                            Verb = "runas"
                        };

                        Process.Start(startInfo);
                    }
                    catch
                    {
                        // ignored
                    }
                }

                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        /// <summary>
        /// Fail the install if the Agent configuration directory exists but is not owned by
        /// Administrators or SYSTEM.
        /// </summary>
        /// <remarks>
        /// Runs before the install makes any change, so it can report the problem without leaving a
        /// partial installation behind. DDCreateFolders applies the same check when it creates the
        /// directory.
        ///
        /// When calledFromUIControl is true the outcome is reported through ConfigRootValidProperty and
        /// ErrorModal_ErrorMessage instead, and the action returns success: a custom action run from a
        /// dialog cannot send a message to the user, and returning failure exits the installer.
        /// https://learn.microsoft.com/en-us/windows/win32/msi/sending-messages-to-windows-installer-using-msiprocessmessage
        /// </remarks>
        internal static ActionResult EnsureSecureConfigRoot(ISession session, bool calledFromUIControl = false)
        {
            if (calledFromUIControl)
            {
                // reset output properties, the user can go back and forth in the UI
                session[ConfigRootValidProperty] = "False";
                session["ErrorModal_ErrorMessage"] = "";
            }

            // Read outside the try so that the messages below can name the directory
            var configRoot = session.Property("APPLICATIONDATADIRECTORY");

            if (string.IsNullOrEmpty(configRoot))
            {
                // On a fresh install or a maintenance (Change/Repair) run, CostFinalize resolves this
                // property to a concrete path before this action runs, so an empty value means the
                // install state that records the configuration directory is unavailable. That happens
                // when operating on an install whose registry state was removed or corrupted - for
                // example uninstalling it. There is no directory to verify or secure in that case, and
                // returning Failure would fail the MSI (exit code 1603) and make a broken-but-removable
                // install impossible to uninstall. Treat it as nothing to verify and let the operation
                // proceed. See incident 58787.
                session.Log("APPLICATIONDATADIRECTORY is not set, skipping the configuration directory owner check");
                if (calledFromUIControl)
                {
                    session[ConfigRootValidProperty] = "True";
                }

                return ActionResult.Success;
            }

            try
            {
                SecureDirectory.AssertSecureOwner(session, configRoot);
            }
            catch (SecureDirectoryException e)
            {
                if (calledFromUIControl)
                {
                    session.Log(e.Message);
                    // Not escaped: the dialog displays this through [ErrorModal_ErrorMessage], and the
                    // value substituted for a property is inserted as it is. Only a message that
                    // becomes a format string itself needs escaping, see ISession.LogAndDisplayError.
                    session["ErrorModal_ErrorMessage"] = e.Message;
                    return ActionResult.Success;
                }

                session.LogAndDisplayError(e.Message);
                return ActionResult.Failure;
            }
            catch (Exception e)
            {
                session.Log($"Failed to verify the configuration directory \"{configRoot}\": {e}");
                if (calledFromUIControl)
                {
                    session["ErrorModal_ErrorMessage"] =
                        $"Failed to verify the configuration directory \"{configRoot}\": {e.Message}";
                    return ActionResult.Success;
                }

                return ActionResult.Failure;
            }

            if (calledFromUIControl)
            {
                session[ConfigRootValidProperty] = "True";
            }

            return ActionResult.Success;
        }

        public static ActionResult EnsureSecureConfigRoot(Session session)
        {
            return EnsureSecureConfigRoot(new SessionWrapper(session));
        }

        public static ActionResult EnsureSecureConfigRootUI(Session session)
        {
            return EnsureSecureConfigRoot(new SessionWrapper(session), calledFromUIControl: true);
        }
    }
}
