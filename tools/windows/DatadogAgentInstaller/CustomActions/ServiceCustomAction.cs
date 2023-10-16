using System;
using System.Linq;
using System.Security.Principal;
using System.ServiceProcess;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;
using Microsoft.Win32;
using ServiceController = Datadog.CustomActions.Native.ServiceController;

namespace Datadog.CustomActions
{
    public class ServiceCustomAction
    {
        private readonly ISession _session;
        private readonly INativeMethods _nativeMethods;
        private readonly IRegistryServices _registryServices;
        private readonly IDirectoryServices _directoryServices;
        private readonly IFileServices _fileServices;
        private readonly IServiceController _serviceController;

        public ServiceCustomAction(
            ISession session,
            INativeMethods nativeMethods,
            IRegistryServices registryServices,
            IDirectoryServices directoryServices,
            IFileServices fileServices,
            IServiceController serviceController)
        {
            _session = session;
            _nativeMethods = nativeMethods;
            _registryServices = registryServices;
            _directoryServices = directoryServices;
            _fileServices = fileServices;
            _serviceController = serviceController;
        }

        public ServiceCustomAction(ISession session)
        : this(
            session,
            new Win32NativeMethods(),
            new RegistryServices(),
            new DirectoryServices(),
            new FileServices(),
            new ServiceController()
        )
        {
        }

        private static ActionResult EnsureNpmServiceDependendency(ISession session)
        {
            try
            {
                using var systemProbeDef = Registry.LocalMachine.OpenSubKey(@"SYSTEM\CurrentControlSet\Services\datadog-system-probe", true);
                if (systemProbeDef != null)
                {
                    // Remove the dependency between "datadog-system-probe" and "ddnpm" services since
                    // the "datadog-system-probe" service now takes care of starting the "ddnpm" service directly.
                    systemProbeDef.SetValue("DependOnService", new[]
                    {
                        Constants.AgentServiceName
                    }, RegistryValueKind.MultiString);
                }
                else
                {
                    session.Log("Registry key does not exist");
                }
            }
            catch (Exception e)
            {
                session.Log($"Could not update system probe dependent service: {e}");
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult EnsureNpmServiceDependency(Session session)
        {
            return EnsureNpmServiceDependendency(new SessionWrapper(session));
        }

        private ActionResult ConfigureServiceUsers()
        {
            try
            {
                // Lookup account so we can determine how to set the password according to the ChangeServiceConfig rules.
                // https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-changeserviceconfigw
                var ddAgentUserName = $"{_session.Property("DDAGENTUSER_PROCESSED_FQ_NAME")}";
                var userFound = _nativeMethods.LookupAccountName(ddAgentUserName,
                    out _,
                    out _,
                    out var securityIdentifier,
                    out _);
                if (!userFound)
                {
                    throw new Exception($"Could not find user {ddAgentUserName}.");
                }

                var ddAgentUserPassword = _session.Property("DDAGENTUSER_PROCESSED_PASSWORD");
                var isServiceAccount = _nativeMethods.IsServiceAccount(securityIdentifier);
                if (!isServiceAccount && string.IsNullOrEmpty(ddAgentUserPassword))
                {
                    _session.Log("Password not provided, will not change service user password");
                    // set to null so we don't modify the service config
                    ddAgentUserPassword = null;
                }
                else if (isServiceAccount)
                {
                    _session.Log("Ignoring provided password because account is a service account");
                    // Follow rules for ChangeServiceConfig
                    if (securityIdentifier.IsWellKnown(WellKnownSidType.LocalSystemSid) ||
                        securityIdentifier.IsWellKnown(WellKnownSidType.LocalServiceSid) ||
                        securityIdentifier.IsWellKnown(WellKnownSidType.NetworkServiceSid))
                    {
                        // Specify an empty string if the account has no password or if the service runs in the LocalService, NetworkService, or LocalSystem account.
                        ddAgentUserPassword = "";
                    }
                    else
                    {
                        // If the account name specified by the lpServiceStartName parameter is the name of a managed service account or virtual account name, the lpPassword parameter must be NULL.
                        ddAgentUserPassword = null;
                    }
                }

                _session.Log($"Configuring services with account {ddAgentUserName}");

                // ddagentuser
                if (securityIdentifier.IsWellKnown(WellKnownSidType.LocalSystemSid))
                {
                    ddAgentUserName = "LocalSystem";
                }
                else if (securityIdentifier.IsWellKnown(WellKnownSidType.LocalServiceSid))
                {
                    ddAgentUserName = "LocalService";
                }
                else if (securityIdentifier.IsWellKnown(WellKnownSidType.NetworkServiceSid))
                {
                    ddAgentUserName = "NetworkService";
                }
                _serviceController.SetCredentials(Constants.AgentServiceName, ddAgentUserName, ddAgentUserPassword);
                _serviceController.SetCredentials(Constants.TraceAgentServiceName, ddAgentUserName, ddAgentUserPassword);

                // SYSTEM
                // LocalSystem is a SCM specific shorthand that doesn't need to be localized
                _serviceController.SetCredentials(Constants.SystemProbeServiceName, "LocalSystem", "");
                _serviceController.SetCredentials(Constants.ProcessAgentServiceName, "LocalSystem", "");

                 var installCWS = _session.Property("INSTALL_CWS");
                 if(!string.IsNullOrEmpty(installCWS)){
                    _serviceController.SetCredentials(Constants.SecurityAgentServiceName, ddAgentUserName, ddAgentUserPassword);
                 }
            }
            catch (Exception e)
            {
                _session.Log($"Failed to configure service logon users: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ConfigureServiceUsers(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session)).ConfigureServiceUsers();
        }

        /// <summary>
        /// Stop any existing datadog services
        /// </summary>
        /// <returns></returns>
        private ActionResult StopDDServices(bool continueOnError)
        {
            try
            {
#if false
                try
                {
                    // Kill MMC as being open can keep a handle on the DatadogAgent service
                    // which will cause a cryptic 1923 error later in the InstallServices phase.
                    // We do this here as opposed to using Wix's CloseApplication feature because this custom action
                    // runs on uninstall.
                    var mmcProcesses = Process.GetProcessesByName("mmc");
                    if (mmcProcesses.Any())
                    {
                        _session.Log("The installer detected that at least one mmc.exe process is currently running. " +
                                     "The mmc.exe process can keep a handle on the Datadog Agent service and cause error 1923 " +
                                     "while trying to install/upgrade the Datadog Agent services. The installer will now attempt to " +
                                     "close those processes.");
                    }
                    foreach (var mmcProc in mmcProcesses)
                    {
                        try
                        {
                            _session.Log($"Found {mmcProc.ProcessName} (PID:{mmcProc.Id}), attempting to kill it.");
                            mmcProc.Kill();
                        }
                        finally
                        {
                            // Don't fail
                        }
                    }
                }
                finally
                {
                    // Don't fail
                }
#endif

                // Stop each service individually in case the install is broken
                // e.g. datadogagent doesn't exist or the service dependencies are not correect.
                //
                // ** some services are optionally included in the package at build time.  Including
                // them here will simply cause a spurious "Service X not found" in the log if the
                // installer is built without that component.
                var ddservices = new []
                {
                    Constants.SystemProbeServiceName,
                    Constants.NpmServiceName,
                    Constants.ProcmonServiceName,       // might not exist depending on compile time options**
                    Constants.SecurityAgentServiceName, // might not exist depending on compile time options**
                    Constants.ProcessAgentServiceName,
                    Constants.TraceAgentServiceName,
                    Constants.AgentServiceName
                };
                foreach (var service in ddservices)
                {
                    try
                    {
                        var svc = _serviceController.Services.FirstOrDefault(svc => svc.ServiceName == service);
                        if (svc != null)
                        {
                            _session.Log($"Service {service} status: {svc.Status}");
                            if (svc.Status == ServiceControllerStatus.Stopped)
                            {
                                // Service is already stopped
                                continue;
                            }
                            using var actionRecord = new Record(
                                "Stop Datadog services",
                                $"Stopping {svc.DisplayName} service",
                                ""
                            );
                            _session.Message(InstallMessage.ActionStart, actionRecord);
                            _session.Log($"Stopping service {service}");
                            _serviceController.StopService(service, TimeSpan.FromMinutes(3));

                            // Refresh to get new status
                            svc.Refresh();
                            _session.Log($"Service {service} status: {svc.Status}");
                        }
                        else
                        {
                            _session.Log($"Service {service} not found");
                        }
                    }
                    catch (Exception e)
                    {
                        if (!continueOnError)
                        {
                            // rethrow exception implicitly to preserve the original error information
                            throw new Exception($"Failed to stop service {service}", e);
                        }
                        _session.Log($"Failed to stop service {service} due to exception {e}\r\n" +
                                     "but will be translated to success due to continue on error.");
                    }
                }
            }
            catch (Exception e)
            {
                _session.Log($"Failed to stop services: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult StopDDServices(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session)).StopDDServices(false);
        }

        private ActionResult StartDDServices()
        {
            try
            {
                using var actionRecord = new Record(
                    "Start Datadog services",
                    "Starting Datadog Agent service",
                    ""
                );
                _session.Message(InstallMessage.ActionStart, actionRecord);
                // only start the main agent service. it should start any other services
                // that should be running.
                _serviceController.StartService(Constants.AgentServiceName, TimeSpan.FromMinutes(3));
            }
            catch (Exception e)
            {
                _session.Log($"Failed to stop services: {e}");
                // Allow service start to fail and continue the install
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult StartDDServices(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session)).StartDDServices();
        }

        private ActionResult StartDDServicesRollback()
        {
            // rollback StartDDServices by stopping all of the services
            // continue on error so we can send a stop to all services.
            return StopDDServices(true);
        }

        [CustomAction]
        public static ActionResult StartDDServicesRollback(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session)).StartDDServicesRollback();
        }
    }
}
