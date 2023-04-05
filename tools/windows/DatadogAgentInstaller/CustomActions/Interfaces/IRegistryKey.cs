using System.Security.AccessControl;

namespace Datadog.CustomActions.Interfaces
{
    public interface IRegistryKey
    {
        void SetAccessControl(RegistrySecurity registrySecurity);
    }
}
