using System;
using System.Windows.Forms;
using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions
{
    public class MsiLogCustomActions
    {
        private static ActionResult OpenMsiLog(ISession session)
        {
            // The MessageBoxs are unlikely to ever show.
            // In testing I couldn't hit the "failed" ones. For permissions errors,
            // Notepad still opens, and Notepad shows its own error dialog box.
            // The log file should always exist because we set the MsiLogging
            // property, so unless that changes we won't see the "no log file"
            // MessageBoxs either.
            // The log file can't be deleted/renamed while the installer is running
            // because the installer has a handle to it.
            // We use MessageBoxIcon.Warning rather than MessageBoxIcon.Error
            // to match the WiX built-in error dialogs.
            var wixLogLocation = string.Empty;
            var messageBoxTitle = "Datadog Agent Setup";
            try
            {
                wixLogLocation = session["MsiLogFileLocation"];
                if (!string.IsNullOrEmpty(wixLogLocation))
                {
                    var proc = System.Diagnostics.Process.Start(wixLogLocation);
                    if (proc == null)
                    {
                        // Did not start a process
                        MessageBox.Show($"Failed to open log file: {wixLogLocation}",
                            messageBoxTitle,
                            MessageBoxButtons.OK,
                            MessageBoxIcon.Warning);
                    }
                }
                else
                {
                    // Log file path property is empty
                    MessageBox.Show("There is no log file. Please pass the /l or /log options to the installer to create a log file.",
                        messageBoxTitle,
                        MessageBoxButtons.OK,
                        MessageBoxIcon.Warning);
                }
            }
            catch (Exception e)
            {
                if (!string.IsNullOrEmpty(wixLogLocation))
                {
                    MessageBox.Show($"Failed to open log file: {wixLogLocation}\n{e.Message}",
                        messageBoxTitle,
                        MessageBoxButtons.OK,
                        MessageBoxIcon.Warning);
                }
                else
                {
                    // Log file path property is empty
                    MessageBox.Show("There is no log file. Please pass the /l or /log options to the installer to create a log file.",
                        messageBoxTitle,
                        MessageBoxButtons.OK,
                        MessageBoxIcon.Warning);
                }
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult OpenMsiLog(Session session)
        {
            return OpenMsiLog(new SessionWrapper(session));
        }
    }
}
