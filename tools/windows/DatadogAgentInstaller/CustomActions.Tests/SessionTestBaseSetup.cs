using System.Collections.Generic;
using Datadog.CustomActions.Interfaces;
using Moq;

namespace CustomActions.Tests
{
    public abstract class SessionTestBaseSetup
    {
        private readonly Dictionary<string, string> _properties = new();

        public IReadOnlyDictionary<string, string> Properties => _properties;

        public Mock<ISession> Session { get; } = new();

        protected SessionTestBaseSetup()
        {
            Session
                .SetupSet(s => s[It.IsAny<string>()] = It.IsAny<string>())
                .Callback((string key, string value) => _properties[key] = value);
            Session
                .Setup(s => s[It.IsAny<string>()])
                .Returns((string key) => _properties.ContainsKey(key) ? _properties[key] : null);
        }
    }
}
