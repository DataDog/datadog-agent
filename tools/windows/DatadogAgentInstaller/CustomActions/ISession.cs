using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions
{
    public interface ISession
    {
        /// <summary>
        /// see <see cref="Session.this[string]"/>
        /// </summary>
        string this[string property]
        {
            get;
            set;
        }

        /// <summary>
        /// see <see cref="Session.Message"/>
        /// </summary>
        MessageResult Message(InstallMessage messageType, Record record);

        /// <summary>
        /// see <see cref="Session.Log(string)"/>
        /// </summary>
        void Log(string msg);

        /// <summary>
        /// see <see cref="Session.Log(string, object[])"/>
        /// </summary>
        void Log(string format, params object[] args);

        /// <summary>
        /// see <see cref="Session.Components"/>
        /// </summary>
        ComponentInfoCollection Components
        {
            get;
        }

        /// <summary>
        /// see <see cref="Session.Features"/>
        /// </summary>
        FeatureInfoCollection Features
        {
            get;
        }

        /// <summary>
        /// see <see cref="Session.CustomActionData"/>
        /// </summary>
        CustomActionData CustomActionData
        {
            get;
        }
    }

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

        public void Log(string msg)
        {
            _session.Log(msg);
        }

        public void Log(string format, params object[] args)
        {
            _session.Log(format, args);
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
