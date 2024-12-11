using System;
using System.Linq;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Native
{
    public static class ServiceControllerExtensions
    {
        public static bool ServiceExists(this IServiceController serviceController, string serviceName)
        {
            return serviceController.Find(serviceName) != null;
        }

        public static IWindowsService Find(this IServiceController serviceController, string serviceName)
        {
            return serviceController
                .Services
                .FirstOrDefault(svc => svc.ServiceName.Equals(serviceName, StringComparison.InvariantCultureIgnoreCase));
        }
    }
}
