using Datadog.CustomActions.Interfaces;
using Microsoft.Win32;

namespace Datadog.CustomActions.Native
{
    class RegistryServices : IRegistryServices
    {
        public IRegistryKey CreateRegistryKey(Registries registry, string path)
        {
            var key = registry switch
            {
                Registries.LocalMachine => Registry.LocalMachine,
                _ => null
            };

            return new RegistryKey(key.CreateSubKey(path));
        }

        public IRegistryKey OpenRegistryKey(Registries registry, string path)
        {
            return OpenRegistryKey(registry, path, false);
        }

        public IRegistryKey OpenRegistryKey(Registries registry, string path, bool writable)
        {
            var key = registry switch
            {
                Registries.LocalMachine => Registry.LocalMachine,
                _ => null
            };

            return new RegistryKey(key.OpenSubKey(path, writable));
        }
    }
}
