using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Diagnostics;

namespace Datadog.CustomActions
{
    /// <summary>
    /// Wraps the <see cref="Session"/> from the Windows installer.
    /// </summary>
    /// This allows the session object to be mocked for unit testing
    /// and also to modify the logging behavior to inject timestamps
    /// and other useful information.
    public class SessionWrapper : ISession
    {
        private readonly Session _session;

        public SessionWrapper(Session session)
        {
            _session = session;
        }

        public string this[string property]
        {
            get => _session[property];
            set => _session[property] = value;
        }

        public MessageResult Message(InstallMessage messageType, Record record)
        {
            return _session.Message(messageType, record);
        }

        public void Log(string msg, string memberName = null, string filePath = null, int lineNumber = 0)
        {
            _session.Log($"CA: {DateTime.Now:HH:mm:ss}: {memberName}. {msg}");
        }

        public ComponentInfoCollection Components => _session.Components;

        public FeatureInfoCollection Features => _session.Features;

        public IFeatureInfo Feature(string FeatureName)
        {
            return new FeatureInfoAdapter(_session.Features[FeatureName]);
        }

        /// <summary>
        /// Runs command and logs stdout/stderr to MSI log, returns the process object.
        /// </summary>
        public Process RunCommand(string filename, string arguments)
        {
            var psi = new ProcessStartInfo
            {
                CreateNoWindow = true,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                FileName = filename,
                Arguments = arguments
            };
            var proc = new Process();
            proc.StartInfo = psi;
            proc.OutputDataReceived += (_, args) => Log(args.Data);
            proc.ErrorDataReceived += (_, args) => Log(args.Data);
            proc.Start();
            proc.BeginOutputReadLine();
            proc.BeginErrorReadLine();
            proc.WaitForExit();
            return proc;
        }

        public CustomActionData CustomActionData => _session.CustomActionData;
    }
}
