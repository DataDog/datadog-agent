using Datadog.CustomActions.Native;
using System;
using System.Collections.Generic;
using System.Security.AccessControl;

namespace Datadog.CustomActions.Interfaces
{
    public interface IServiceController
    {
        IReadOnlyList<IWindowsService> Services { get; }
        void SetCredentials(string serviceName, string username, string password);
        void StopService(string serviceName, TimeSpan timeout);
        void StartService(string serviceName, TimeSpan timeout);
        public CommonSecurityDescriptor GetAccessSecurity(string serviceName);
        public void SetAccessSecurity(string serviceName, CommonSecurityDescriptor sd);
    }
}
