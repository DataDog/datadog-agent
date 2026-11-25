using Datadog.CustomActions.Native;
using System.Security.AccessControl;

namespace Datadog.CustomActions.Interfaces
{
    public interface IRegistryServices
    {
        IRegistryKey CreateRegistryKey(Registries registry, string path);
        IRegistryKey OpenRegistryKey(Registries registry, string path);
        IRegistryKey OpenRegistryKey(Registries registry, string path, bool writable);
        IRegistryKey OpenRegistryKey(Registries registry, string path, RegistryRights rights);
        void DeleteSubKey(Registries registry, string path);
    }
}
