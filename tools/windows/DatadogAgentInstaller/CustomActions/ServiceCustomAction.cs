using System;
using System.Collections.Generic;
using System.Linq;
using System.Security.AccessControl;
using System.Security.Authentication.ExtendedProtection;
using System.Security.Principal;
using System.ServiceProcess;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
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
        private readonly IFileSystemServices _fileSystemServices;

        private readonly RollbackDataStore _rollbackDataStore;

        public ServiceCustomAction(
            ISession session,
            string rollbackDataName,
            INativeMethods nativeMethods,
            IRegistryServices registryServices,
            IDirectoryServices directoryServices,
            IFileServices fileServices,
            IServiceController serviceController,
            IFileSystemServices fileSystemServices)
        {
            _session = session;
            _nativeMethods = nativeMethods;
            _registryServices = registryServices;
            _directoryServices = directoryServices;
            _fileServices = fileServices;
            _serviceController = serviceController;
            _fileSystemServices = fileSystemServices;

            if (!string.IsNullOrEmpty(rollbackDataName))
            {
                _rollbackDataStore = new RollbackDataStore(_session, rollbackDataName, _fileSystemServices, _serviceController);
            }
        }

        public ServiceCustomAction(ISession session)
            : this(
                session,
                null,
                new Win32NativeMethods(),
                new RegistryServices(),
                new DirectoryServices(),
                new FileServices(),
                new ServiceController(),
                new FileSystemServices()
            )
        {
        }

        public ServiceCustomAction(ISession session, string rollbackDataName)
        : this(
            session,
            rollbackDataName,
            new Win32NativeMethods(),
            new RegistryServices(),
            new DirectoryServices(),
            new FileServices(),
            new ServiceController(),
            new FileSystemServices()
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
        private ActionResult ConfigureServices()
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

                ConfigureServiceUsers(ddAgentUserName, securityIdentifier);
                ConfigureServicePermissions(securityIdentifier);
            }
            catch (Exception e)
            {
                _session.Log($"Failed to configure services: {e}");
                return ActionResult.Failure;
            }
            finally
            {
                _rollbackDataStore.Store();
            }
            return ActionResult.Success;
        }

        private void ConfigureServiceUsers(string ddAgentUserName, SecurityIdentifier ddAgentUserSID)
        {
            var ddAgentUserPassword = _session.Property("DDAGENTUSER_PROCESSED_PASSWORD");
            var isServiceAccount = _nativeMethods.IsServiceAccount(ddAgentUserSID);
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
                if (ddAgentUserSID.IsWellKnown(WellKnownSidType.LocalSystemSid) ||
                    ddAgentUserSID.IsWellKnown(WellKnownSidType.LocalServiceSid) ||
                    ddAgentUserSID.IsWellKnown(WellKnownSidType.NetworkServiceSid))
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
            if (ddAgentUserSID.IsWellKnown(WellKnownSidType.LocalSystemSid))
            {
                ddAgentUserName = "LocalSystem";
            }
            else if (ddAgentUserSID.IsWellKnown(WellKnownSidType.LocalServiceSid))
            {
                ddAgentUserName = "LocalService";
            }
            else if (ddAgentUserSID.IsWellKnown(WellKnownSidType.NetworkServiceSid))
            {
                ddAgentUserName = "NetworkService";
            }
            _serviceController.SetCredentials(Constants.AgentServiceName, ddAgentUserName, ddAgentUserPassword);
            _serviceController.SetCredentials(Constants.TraceAgentServiceName, ddAgentUserName, ddAgentUserPassword);

            // SYSTEM
            // LocalSystem is a SCM specific shorthand that doesn't need to be localized
            _serviceController.SetCredentials(Constants.SystemProbeServiceName, "LocalSystem", "");
            _serviceController.SetCredentials(Constants.ProcessAgentServiceName, "LocalSystem", "");

            _serviceController.SetCredentials(Constants.SecurityAgentServiceName, ddAgentUserName, ddAgentUserPassword);
        }

        private void UpdateAndLogAccessControl(string serviceName, CommonSecurityDescriptor securityDescriptor)
        {
            var oldSD = _serviceController.GetAccessSecurity(serviceName);
            _session.Log(
                $"{serviceName} current ACLs: {oldSD.GetSddlForm(AccessControlSections.All)}");

            _rollbackDataStore.Add(new ServicePermissionRollbackData(serviceName, _serviceController));
            _serviceController.SetAccessSecurity(serviceName, securityDescriptor);

            var newSD = _serviceController.GetAccessSecurity(serviceName);
            _session.Log(
                $"{serviceName} new ACLs: {newSD.GetSddlForm(AccessControlSections.All)}");
        }

        /// <summary>
        /// Grant ddagentuser start/stop service privileges for the agent services
        /// </summary>
        private void ConfigureServicePermissions(SecurityIdentifier ddAgentUserSID)
        {
            var previousDdAgentUserSid = InstallStateCustomActions.GetPreviousAgentUser(_session, _registryServices, _nativeMethods);

            var services = new List<string>
            {
                Constants.ProcessAgentServiceName,
                Constants.SystemProbeServiceName,
                Constants.TraceAgentServiceName,
                Constants.AgentServiceName,
            };

            services.Add(Constants.SecurityAgentServiceName);

            foreach (var serviceName in services)
            {
                var securityDescriptor = _serviceController.GetAccessSecurity(serviceName);

                // remove previous user
                if (previousDdAgentUserSid != null && previousDdAgentUserSid != ddAgentUserSID)
                {
                    // unless that user is LocalSystem
                    if (!previousDdAgentUserSid.IsWellKnown(WellKnownSidType.LocalSystemSid))
                    {
                        securityDescriptor.DiscretionaryAcl.RemoveAccess(AccessControlType.Allow,
                            previousDdAgentUserSid,
                            (int)(ServiceAccess.SERVICE_ALL_ACCESS), InheritanceFlags.None, PropagationFlags.None);
                    }
                }

                // Remove Everyone
                // [7.47 - 7.50) added an ACE for Everyone, so make sure to remove it
                securityDescriptor.DiscretionaryAcl.RemoveAccess(AccessControlType.Allow, new SecurityIdentifier("WD"),
                    (int)(ServiceAccess.SERVICE_ALL_ACCESS), InheritanceFlags.None, PropagationFlags.None);

                // add current user
                // Unless the user is LocalSystem since it already has access
                if (!ddAgentUserSID.IsWellKnown(WellKnownSidType.LocalSystemSid))
                {
                    securityDescriptor.DiscretionaryAcl.AddAccess(AccessControlType.Allow, ddAgentUserSID,
                        (int)(ServiceAccess.SERVICE_START | ServiceAccess.SERVICE_STOP | ServiceAccess.GENERIC_READ),
                        InheritanceFlags.None, PropagationFlags.None);
                }

                UpdateAndLogAccessControl(serviceName, securityDescriptor);
            }
        }

        [CustomAction]
        public static ActionResult ConfigureServices(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session), "ConfigureServices").ConfigureServices();
        }

        private ActionResult ConfigureServicesRollback()
        {
            try
            {
                _rollbackDataStore.Restore();
            }
            catch (Exception e)
            {
                _session.Log($"Failed to rollback service configuration: {e}");
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ConfigureServicesRollback(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session), "ConfigureServices").ConfigureServicesRollback();
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
                var ddservices = new[]
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
