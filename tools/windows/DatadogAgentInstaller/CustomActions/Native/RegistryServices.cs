using Datadog.CustomActions.Interfaces;
using Microsoft.Win32;

namespace Datadog.CustomActions.Native
{
    class RegistryServices : IRegistryServices
    {
        public IRegistryKey CreateRegistryKey(Registries registry, string path)
        {
            Microsoft.Win32.RegistryKey key = null;
            switch (registry)
            {
                case Registries.LocalMachine:
                    key = Registry.LocalMachine;
                    break;
                case Registries.Users:
                    key = Registry.Users;
                    break;
                case Registries.ClassesRoot:
                    key = Registry.ClassesRoot;
                    break;
                case Registries.CurrentConfig:
                    key = Registry.CurrentConfig;
                    break;
                case Registries.CurrentUser:
                    key = Registry.CurrentUser;
                    break;
                case Registries.PerformanceData:
                    key = Registry.PerformanceData;
                    break;
            }

            return new RegistryKey(key.CreateSubKey(path));
        }
    }
}
