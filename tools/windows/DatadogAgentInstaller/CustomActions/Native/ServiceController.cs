using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Linq;
using System.ServiceProcess;
using System.Threading;
using System.Threading.Tasks;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Native
{
    class ServiceController : IServiceController
    {
        public IReadOnlyList<IWindowsService> Services
        {
            get
            {
                var services = System.ServiceProcess
                    .ServiceController
                    .GetServices();
                // .NET hides device driver services behind a separate API, we combine them here
                services = services.Concat(System.ServiceProcess
                    .ServiceController
                    .GetDevices()).ToArray();
                return services
                    .Select(svc => new WindowsService(svc))
                    .ToList();
            }
        }

        public Tuple<string,string>[] GetServiceNames()
        {
            return Services
                .Select(svc => Tuple.Create(svc.ServiceName,svc.DisplayName))
                .ToArray();
        }

        public bool ServiceExists(string serviceName)
        {
            return Services
                .Any(svc => svc.ServiceName.Equals(serviceName, StringComparison.InvariantCultureIgnoreCase));
        }

        public ServiceControllerStatus? ServiceStatus(string serviceName)
        {
            return Services
                .FirstOrDefault(svc => svc.ServiceName.Equals(serviceName, StringComparison.InvariantCultureIgnoreCase))
                ?.Status;
        }

        public void SetCredentials(string serviceName, string username, string password)
        {
            var svc = new System.ServiceProcess.ServiceController(serviceName);
            if (!Win32NativeMethods.ChangeServiceConfig(svc.ServiceHandle,
                (uint)Win32NativeMethods.SERVICE_NO_CHANGE,
                Win32NativeMethods.SERVICE_NO_CHANGE,
                Win32NativeMethods.SERVICE_NO_CHANGE,
                null,
                null,
                null,
                null,
                username,
                password,
                null))
            {
                throw new Win32Exception($"ChangeServiceConfig({serviceName}) failed");
            }
        }

        private async Task<ServiceControllerStatus> WaitForStatusChange(System.ServiceProcess.ServiceController svc, ServiceControllerStatus state, TimeSpan timeout)
        {
            using var cts = new CancellationTokenSource(timeout);
            var delay = TimeSpan.FromMilliseconds(250);
            while (!cts.IsCancellationRequested)
            {
                svc.Refresh();
                if (svc.Status != state)
                {
                    return svc.Status;
                }

                // Use the same interval as ServiceController.WaitForStatus
                await Task.Delay(delay, cts.Token);
            }
            throw new System.ServiceProcess.TimeoutException();
        }

        /// <summary>
        /// Waits at most timeout for serviceName to enter the Stopped state
        /// </summary>
        /// <param name="serviceName"></param>
        /// <param name="timeout"></param>
        public void StopService(string serviceName, TimeSpan timeout)
        {
            var svc = new System.ServiceProcess.ServiceController(serviceName);
            if (!(svc.Status == ServiceControllerStatus.Stopped || svc.Status == ServiceControllerStatus.StopPending))
            {
                svc.Stop();
            }
            svc.WaitForStatus(ServiceControllerStatus.Stopped, timeout);
        }

        /// <summary>
        /// Waits at most timeout for serviceName to be running or to fail to start
        /// </summary>
        /// <param name="serviceName"></param>
        /// <param name="timeout"></param>
        /// <exception cref="Exception"></exception>
        public void StartService(string serviceName, TimeSpan timeout)
        {
            var svc = new System.ServiceProcess.ServiceController(serviceName);
            if (!(svc.Status == ServiceControllerStatus.Running || svc.Status == ServiceControllerStatus.StartPending))
            {
                svc.Start();
            }
            // svc.Start() puts the service into the StartPending state before returning.
            // The service will either succeed to start, or fail to start, so if we use
            // WaitForStatus to wait for a single state then we'll block for timeout
            // in the other case. So instead we wait for the service status to change.
            // https://learn.microsoft.com/en-us/windows/win32/services/starting-a-service
            var newState = WaitForStatusChange(svc, ServiceControllerStatus.StartPending, timeout).Result;
            if (newState != ServiceControllerStatus.Running)
            {
                throw new Exception($"Failed to start {serviceName} service");
            }
        }
    }
}
