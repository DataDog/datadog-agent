using System;
using System.Configuration;
using System.Diagnostics;
using System.Linq;
using System.Runtime.InteropServices;
using System.ServiceProcess;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Native
{
    class ServiceController : IServiceController
    {
        public Tuple<string,string>[] GetServiceNames()
        {
            return System.ServiceProcess.ServiceController.GetServices()
                .Select(svc => Tuple.Create(svc.ServiceName,svc.DisplayName))
                .ToArray();
        }

        public bool ServiceExists(string serviceName)
        {
            var svc = System.ServiceProcess.ServiceController.GetServices().FirstOrDefault(svc => svc.ServiceName == serviceName);
            return svc != null;
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
                throw new System.Exception($"ChangeServiceConfig({serviceName}) failed {Marshal.GetLastWin32Error()}");
            }
        }

        private ServiceControllerStatus WaitForStatusChange(System.ServiceProcess.ServiceController svc, ServiceControllerStatus state, TimeSpan timeout)
        {
            var timer = new Stopwatch();
            timer.Start();

            while (true)
            {
                svc.Refresh();
                if (svc.Status != state)
                {
                    return svc.Status;
                }
                if (timer.Elapsed > timeout)
                {
                    throw new System.ServiceProcess.TimeoutException();
                }
                // Use the same interval as ServiceController.WaitForStatus
                System.Threading.Thread.Sleep(250);
            }
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
            var newState = WaitForStatusChange(svc, ServiceControllerStatus.StartPending, timeout);
            if (newState != ServiceControllerStatus.Running)
            {
                throw new Exception($"Failed to start {serviceName} service");
            }
        }
    }
}
