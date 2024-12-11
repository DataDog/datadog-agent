using System;
using System.Security.AccessControl;
using Microsoft.Win32;

namespace Datadog.CustomActions.Interfaces
{
    public interface IRegistryKey : IDisposable
    {
        void SetAccessControl(RegistrySecurity registrySecurity);
        object GetValue(string name);
        void SetValue(string name, object value, RegistryValueKind kind);
        void DeleteValue(string name);
    }
}
