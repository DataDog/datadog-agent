using System.Security.AccessControl;
using System;
using System.ServiceProcess;
using Datadog.CustomActions.Interfaces;
using Microsoft.Win32;

namespace Datadog.CustomActions.Native
{
    public class WindowsService : IWindowsService
    {
        private readonly System.ServiceProcess.ServiceController _service;

        public WindowsService(System.ServiceProcess.ServiceController service)
        {
            _service = service;
        }

        public string ServiceName => _service.ServiceName;

        public string DisplayName => _service.DisplayName;

        public ServiceControllerStatus Status => _service.Status;

        public ServiceStartMode StartType
        {
            get
            {
                using var key = Registry.LocalMachine.OpenSubKey($@"SYSTEM\CurrentControlSet\Services\{_service.ServiceName}", RegistryKeyPermissionCheck.ReadSubTree, RegistryRights.ReadKey);
                if (key == null)
                {
                    throw new ArgumentException("Invalid request of StartType for non-existent service {_service.ServiceName}");
                }

                if (Enum.TryParse<ServiceStartMode>(key.GetValue("Start").ToString(), out var serviceStartMode))
                {
                    return serviceStartMode;
                }

                throw new Exception($"Unexpected Start value for service {_service.ServiceName}");
            }
        }
        public void Refresh() => _service.Refresh();
    }
}
