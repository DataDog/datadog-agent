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

            key = key.CreateSubKey(path);
            if (key == null)
            {
                return null;
            }

            return new RegistryKey(key);
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

            key = key.OpenSubKey(path, writable);
            if (key == null)
            {
                return null;
            }

            return new RegistryKey(key);
        }
    }
}
