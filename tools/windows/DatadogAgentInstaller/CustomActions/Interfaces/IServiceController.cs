using Datadog.CustomActions.Native;
using System;
using System.Collections.Generic;

namespace Datadog.CustomActions.Interfaces
{
    public interface IServiceController
    {
        IReadOnlyList<IWindowsService> Services { get; }
        void SetCredentials(string serviceName, string username, string password);
        void StopService(string serviceName, TimeSpan timeout);
        void StartService(string serviceName, TimeSpan timeout);
    }
}
