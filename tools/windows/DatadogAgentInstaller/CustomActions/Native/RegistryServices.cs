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
    }
}
