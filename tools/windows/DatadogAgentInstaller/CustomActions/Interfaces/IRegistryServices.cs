using Datadog.CustomActions.Native;

namespace Datadog.CustomActions.Interfaces
{
    public interface IRegistryServices
    {
        IRegistryKey CreateRegistryKey(Registries registry, string path);
    }
}
