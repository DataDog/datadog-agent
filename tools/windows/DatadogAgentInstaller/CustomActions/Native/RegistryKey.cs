using System.Security.AccessControl;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Native
{
    class RegistryKey : IRegistryKey
    {
        private readonly Microsoft.Win32.RegistryKey _key;

        public RegistryKey(Microsoft.Win32.RegistryKey key)
        {
            _key = key;
        }

        public void SetAccessControl(RegistrySecurity registrySecurity)
        {
            _key.SetAccessControl(registrySecurity);
        }

        public object GetValue(string name)
        {
            return _key?.GetValue(name);
        }

        public void Dispose()
        {
            _key?.Dispose();
        }
    }
}
