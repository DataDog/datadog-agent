using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Diagnostics;
using System.IO;
using System.Text;

namespace Datadog.CustomActions
{

    public class PythonDistributionCustomAction
    {
        private readonly ISession _session;
        private readonly IFileSystemServices _fileSystemServices;
        private readonly IServiceController _serviceController;
        private readonly RollbackDataStore _rollbackDataStore;
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

        public PythonDistributionCustomAction(
            ISession session,
            string rollbackDataName,
            IFileSystemServices fileSystemServices,
            IServiceController serviceController)
        {
            _session = session;
            _fileSystemServices = fileSystemServices;
            _serviceController = serviceController;

            _rollbackDataStore = new RollbackDataStore(session, rollbackDataName, _fileSystemServices, _serviceController);
        }

        public PythonDistributionCustomAction(ISession session, string rollbackDataName)
            : this(
                session,
                rollbackDataName,
                new FileSystemServices(),
                new ServiceController()
            )
        {
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

        private static ActionResult RunPythonScript(ISession session, string script)
        {
            var projectLocation = session.Property("PROJECTLOCATION");
            var dataDirectory = session.Property("APPLICATIONDATADIRECTORY");
            // get protected directory path
            dataDirectory = Path.Combine(dataDirectory, "protected");

            var pythonPath = Path.Combine(projectLocation, "embedded3", "python.exe");
            var postInstScript = Path.Combine(projectLocation, "python-scripts", script);
            if (!File.Exists(pythonPath))
            {
                session.Log($"Python executable not found at {pythonPath}");
                return ActionResult.Failure;
            }
            if (!File.Exists(postInstScript))
            {
                session.Log($"install script not found at {postInstScript}");
                return ActionResult.Failure;
            }
            // remove trailing backslash, a backslash followed by a double quote causes the command line to be spit incorrectly 
            projectLocation = projectLocation.TrimEnd('\\');
            dataDirectory = dataDirectory.TrimEnd('\\');

            var psi = new ProcessStartInfo
            {
                CreateNoWindow = true,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                FileName = pythonPath,
                Arguments = $"\"{postInstScript}\" \"{projectLocation}\" \"{dataDirectory}\""
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
                session.Log($"install script exited with code: {proc.ExitCode}");
                proc.Close();
                return ActionResult.Failure;
            }
            proc.Close();
            return ActionResult.Success;
        }

        private ActionResult RunPostInstPythonScript()
        {
            // check if INSTALL_PYTHON_THIRD_PARTY_DEPS property is set
            var installPythonThirdPartyDeps = _session.Property("INSTALL_PYTHON_THIRD_PARTY_DEPS");
            if (string.IsNullOrEmpty(installPythonThirdPartyDeps) || installPythonThirdPartyDeps != "1")
            {
                _session.Log("Skipping installation of third-party Python deps. Set INSTALL_PYTHON_THIRD_PARTY_DEPS=1 to enable this feature.");
                return ActionResult.Success;
            }
            return RunPythonScript(_session, "post.py");

        }

        private ActionResult RunPreRemovePythonScript()
        {
            // add the .post_python_installed_packages.txt to the rollback data store
            var pythonPackagesFile = Path.Combine(_session.Property("APPLICATIONDATADIRECTORY"), "protected", ".post_python_installed_packages.txt");
            // check that file path exists
            if (!System.IO.File.Exists(pythonPackagesFile))
            {
                _session.Log($"File {pythonPackagesFile} does not exist, skipping rollback data store addition.");
            }
            else
            {
                _rollbackDataStore.Add(new FileStorageRollbackData(pythonPackagesFile));
            }

            var result =  RunPythonScript(_session, "pre.py");
            _rollbackDataStore.Store();
            return result;
        }
        private void RunRollbackDataRestore()
        {
             try
            {
                _rollbackDataStore.Restore();
            }
            catch (Exception e)
            {
                _session.Log($"Error while restoring rollback post install file: {e}");
            }
        }

        private ActionResult RunPreRemovePythonScriptRollback()
        {
            RunRollbackDataRestore();
            return ActionResult.Success;
        }
        
        public static ActionResult RunPostInstPythonScript(Session session)
        {
            return new PythonDistributionCustomAction(new SessionWrapper(session), "pythonDistribution").RunPostInstPythonScript();
        }

        public static ActionResult RunPreRemovePythonScript(Session session)
        {
            return new PythonDistributionCustomAction(new SessionWrapper(session), "pythonDistribution").RunPreRemovePythonScript();
        }

        public static ActionResult RunPreRemovePythonScriptRollback(Session session)
        {
            return new PythonDistributionCustomAction(new SessionWrapper(session), "pythonDistribution").RunPreRemovePythonScriptRollback();
        }
    }
}
