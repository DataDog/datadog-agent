using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Diagnostics;
using System.IO;

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
                        "Decompressing Python distribution",
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

                    Process proc = session.RunCommand(
                        Path.Combine(projectLocation, "bin", "7zr.exe"),
                        $"x \"{embedded}\" -o\"{projectLocation}\"");
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

                // delete the files we don't need anymore
                var cleanupFiles = new[] {
                    Path.Combine(projectLocation, "bin", "7zr.exe"),
                    embedded
                };
                foreach (var file in cleanupFiles)
                {
                    try
                    {
                        File.Delete(file);
                    }
                    catch (Exception e)
                    {
                        session.Log($"Error while deleting {file}: {e}");
                        // don't fail if we can't delete the file, it's not critical
                        // and we've seen cases where antivirus may have been holding the file open
                    }
                }
            }
            catch (Exception e)
            {
                session.Log($"Error while decompressing {compressedDistributionFile}: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        /// <summary>
        /// Configure Python's OpenSSL FIPS module
        /// </summary>
        /// <remarks>
        /// The OpenSSL security policy states:
        /// "The Module shall have the self-tests run, and the Module config file output generated on each
        ///  platform where it is intended to be used. The Module config file output data shall not be copied from
        ///  one machine to another."
        /// https://github.com/openssl/openssl/blob/master/README-FIPS.md
        /// </remarks>
        private static ActionResult FinalizeOpenSSLFIPSInstall(ISession session)
        {
            try
            {
                // Installing FIPS flavor, we must run fipsinstall to prepare Python's OpenSSL FIPS module
                using var actionRecord = new Record(
                    "OpenSSL FIPS module installation",
                    "Installing OpenSSL FIPS module",
                    ""
                );
                session.Message(InstallMessage.ActionStart, actionRecord);

                var projectLocation = session.Property("PROJECTLOCATION");
                if (string.IsNullOrEmpty(projectLocation))
                {
                    throw new Exception("PROJECTLOCATION property is not set");
                }
                var embeddedPath = Path.Combine(projectLocation, "embedded3");
                var opensslPath = Path.Combine(embeddedPath, "bin", "openssl.exe");
                var fipsConfPath = Path.Combine(embeddedPath, "ssl", "fipsmodule.cnf");
                var fipsProviderPath = Path.Combine(embeddedPath, "lib", "ossl-modules", "fips.dll");
                var opensslConfPath = Path.Combine(embeddedPath, "ssl", "openssl.cnf");
                var opensslConfTemplate = opensslConfPath + ".tmp";

                // Run fipsinstall command to generate fipsmodule.cnf
                // We provide the -self_test_onload option to ensure that the install-status and install-mac options
                // are NOT written to fipsmodule.cnf, this means the self tests will be run on every Agent start.
                // Being a host install this is not strictly necessary but it is our preference because
                // - it ensures compliance by always running the self tests (consider a golden image deployment scenario)
                // - our container images are built with the same configuration
                // https://docs.openssl.org/master/man5/fips_config
                session.Log("Running openssl fipsinstall");
                using (Process proc = session.RunCommand(
                           opensslPath,
                           $"fipsinstall -module \"{fipsProviderPath}\" -out \"{fipsConfPath}\" -self_test_onload"))
                {
                    if (proc.ExitCode != 0)
                    {
                        throw new Exception($"openssl fipsinstall exited with code: {proc.ExitCode}");
                    }
                }


                // Run again with -verify option
                session.Log("Running openssl fipsinstall -verify");
                using (Process proc = session.RunCommand(
                           opensslPath,
                           $"fipsinstall -module \"{fipsProviderPath}\" -in \"{fipsConfPath}\" -verify"))
                {
                    if (proc.ExitCode != 0)
                    {
                        throw new Exception($"openssl fipsinstall verification of FIPS compliance failed, exited with code: {proc.ExitCode}");
                    }
                }

                // Now we need to update the openssl.cnf file to include the fipsmodule.cnf
                var lines = File.ReadAllLines(opensslConfTemplate);
                using var writer = new StreamWriter(opensslConfPath);
                foreach (var line in lines)
                {
                    if (line.Contains(".include") && line.Contains("fipsmodule.cnf"))
                    {
                        // The template generated at build time includes the default installation path
                        // but the Windows Agent can be installed in custom locations, so we need to replace it.
                        // We must use an absolute path for the .include directive.
                        // https://docs.openssl.org/master/man5/config/#directives
                        // "As a general rule, the pathname should be an absolute path; ... If the pathname is
                        //  still relative, it is interpreted based on the current working directory."
                        // config needs forward slashes for the path
                        var pathForConfig = fipsConfPath.Replace("\\", "/");
                        writer.WriteLine($".include {pathForConfig}");
                    }
                    else
                    {
                        // write line as is
                        writer.WriteLine(line);
                    }
                }
            }
            catch (Exception e)
            {
                session.Log($"Error while finalizing OpenSSL FIPS install: {e}");
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
            ActionResult res = DecompressPythonDistribution(session, "embedded3", "embedded3.COMPRESSED", size);
            if (res != ActionResult.Success)
            {
                return res;
            }

            if (session.Property("AgentFlavor") == Constants.FipsFlavor)
            {
                res = FinalizeOpenSSLFIPSInstall(session);
                if (res != ActionResult.Success)
                {
                    return res;
                }
            }

            return ActionResult.Success;
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
            // This means the file can be restored on failure and still used to create a diff file for next retry
            // Helps prevents silent failure of third party integration feature. Where there is no post file so it silenly continues.
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

            try
            {
                return RunPythonScript(_session, "pre.py");
            }
            finally
            {
                _rollbackDataStore.Store();
            }
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
            return new PythonDistributionCustomAction(new SessionWrapper(session), "pythonPostDistribution").RunPostInstPythonScript();
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
