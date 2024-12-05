using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Diagnostics;
using System.IO;
using System.Text;

namespace Datadog.CustomActions
{
    public class PythonDistributionCustomAction
    {
        public static class MessageRecordFields
        {
            /// <summary>
            /// Resets progress bar and sets the expected total number of ticks in the bar.
            /// </summary>
            public const int MasterReset = 0;

            /// <summary>
            /// Provides information related to progress messages to be sent by the current action.
            /// </summary>
            public const int ActionInfo = 1;

            /// <summary>
            /// Increments the progress bar.
            /// </summary>
            public const int ProgressReport = 1;

            /// <summary>
            /// Enables an action (such as CustomAction) to add ticks to the expected total number of progress of the progress bar.
            /// </summary>
            public const int ProgressAddition = 1;
        }

        private static ActionResult DecompressPythonDistribution(
            ISession session,
            string outputDirectoryName,
            string compressedDistributionFile,
            int pythonDistributionSize)
        {
            var projectLocation = session.Property("PROJECTLOCATION");

            try
            {
                var embedded = Path.Combine(projectLocation, compressedDistributionFile);
                var outputPath = Path.Combine(projectLocation, outputDirectoryName);

                // ensure extract result directory is empty so that we don't merge the directories
                // for different installs. The uninstaller should have already removed/backed up its
                // embedded directories, so this is just in case the uninstaller failed to do so.
                if (Directory.Exists(outputPath))
                {
                    session.Log($"Deleting directory \"{outputPath}\"");
                    Directory.Delete(outputPath, true);
                }
                else
                {
                    session.Log($"{outputPath} not found, skip deletion.");
                }

                if (File.Exists(embedded))
                {
                    using var actionRecord = new Record(
                        "Decompress Python distribution",
                        "Decompressing distribution",
                        ""
                    );
                    session.Message(InstallMessage.ActionStart, actionRecord);

                    {
                        using var record = new Record(MessageRecordFields.ActionInfo,
                            0,  // Number of ticks the progress bar moves for each ActionData message. This field is ignored if Field 3 is 0.
                            0   // The current action will send explicit ProgressReport messages.
                            );
                        session.Message(InstallMessage.Progress, record);
                    }

                    var psi = new ProcessStartInfo
                    {
                        CreateNoWindow = true,
                        UseShellExecute = false,
                        RedirectStandardOutput = true,
                        RedirectStandardError = true,
                        FileName = Path.Combine(projectLocation, "bin", "7zr.exe"),
                        // The archive already contains the name of the embedded folder
                        // so pass the projectLocation to 7z instead.
                        Arguments = $"x \"{embedded}\" -o\"{projectLocation}\""
                    };
                    var proc = new Process();
                    proc.StartInfo = psi;
                    proc.OutputDataReceived += (_, args) => session.Log(args.Data);
                    proc.ErrorDataReceived += (_, args) => session.Log(args.Data);
                    proc.Start();
                    proc.BeginOutputReadLine();
                    proc.BeginErrorReadLine();
                    proc.WaitForExit();
                    if (proc.ExitCode != 0)
                    {
                        throw new Exception($"extracting embedded python exited with code: {proc.ExitCode}");
                    }
                    {
                        using var record = new Record(MessageRecordFields.ProgressReport, pythonDistributionSize);
                        session.Message(InstallMessage.Progress, record);
                    }
                }
                else
                {
                    if (embedded.Contains("embedded3"))
                    {
                        throw new InvalidOperationException($"The file {embedded} doesn't exist, but it should");
                    }
                    session.Log($"{embedded} not found, skipping decompression.");
                }

                File.Delete(Path.Combine(projectLocation, "bin", "7zr.exe"));
                File.Delete(embedded);
            }
            catch (Exception e)
            {
                session.Log($"Error while decompressing {compressedDistributionFile}: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        private static ActionResult DecompressPythonDistributions(ISession session)
        {
            var size = 0;
            var embedded3Size = session.Property("embedded3_SIZE");
            if (!string.IsNullOrEmpty(embedded3Size))
            {
                size = int.Parse(embedded3Size);
            }
            return DecompressPythonDistribution(session, "embedded3", "embedded3.COMPRESSED", size);
        }

        public static ActionResult DecompressPythonDistributions(Session session)
        {
            return DecompressPythonDistributions(new SessionWrapper(session));
        }

        private static ActionResult PrepareDecompressPythonDistributions(ISession session)
        {
            try
            {
                var total = 0;
                var embedded3Size = session.Property("embedded3_SIZE");
                if (!string.IsNullOrEmpty(embedded3Size))
                {
                    total += int.Parse(embedded3Size);
                }
                // Add embedded Python size to the progress bar size
                // Even though we can't record accurate progress, it will look like it's
                // moving every time we decompress a Python distribution.
                using var record = new Record(MessageRecordFields.ProgressAddition, total);
                if (session.Message(InstallMessage.Progress, record) != MessageResult.OK)
                {
                    session.Log("Could not set the progress bar size");
                    return ActionResult.Failure;
                }
            }
            catch (Exception e)
            {
                session.Log($"Error settings the progress bar size: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        public static ActionResult PrepareDecompressPythonDistributions(Session session)
        {
            return PrepareDecompressPythonDistributions(new SessionWrapper(session));
        }
    }
}
