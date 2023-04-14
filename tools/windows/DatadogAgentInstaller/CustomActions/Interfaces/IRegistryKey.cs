using System;
using System.Security.AccessControl;

namespace Datadog.CustomActions.Interfaces
{
    public interface IRegistryKey : IDisposable
    {
        void SetAccessControl(RegistrySecurity registrySecurity);
        object GetValue(string name);
    }
}
