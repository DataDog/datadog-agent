using System.ServiceProcess;
using Datadog.CustomActions.Interfaces;

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

        public ServiceStartMode StartType => _service.StartType;
    }
}
