using Microsoft.Deployment.WindowsInstaller;
using System;

namespace Datadog.CustomActions
{
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

        public void Log(string msg, string memberName, string filePath, int lineNumber)
        {
#if DEBUG
            _session.Log($"[{DateTime.UtcNow:g} - {memberName} - {filePath}:line {lineNumber}] {msg}");
#else
            _session.Log($"[{DateTime.UtcNow:g} - {memberName}] {msg}");
#endif
        }

        public ComponentInfoCollection Components
        {
            get => _session.Components;
        }

        public FeatureInfoCollection Features
        {
            get => _session.Features;
        }

        public CustomActionData CustomActionData => _session.CustomActionData;
    }
}
