using System.ServiceProcess;

namespace Datadog.CustomActions.Interfaces
{
    public interface IWindowsService
    {
        string ServiceName { get; }
        string DisplayName { get; }

        ServiceControllerStatus Status { get; }
        ServiceStartMode StartType { get; }

        void Refresh();
    }
}
