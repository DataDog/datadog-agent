using System;
using System.ServiceProcess;

namespace Datadog.CustomActions.Interfaces
{
    public interface IServiceController
    {
        Tuple<string,string>[] GetServiceNames();
        bool ServiceExists(string serviceName);
        ServiceControllerStatus? ServiceStatus(string serviceName);
        void SetCredentials(string serviceName, string username, string password);
        void StopService(string serviceName, TimeSpan timeout);
        void StartService(string serviceName, TimeSpan timeout);
    }
}
