using System.Linq;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Native
{
    class ServiceController : IServiceController
    {
        public string[] GetServiceNames()
        {
            return System.ServiceProcess.ServiceController.GetServices()
                .Select(svc => svc.ServiceName)
                .ToArray();
        }
    }
}
