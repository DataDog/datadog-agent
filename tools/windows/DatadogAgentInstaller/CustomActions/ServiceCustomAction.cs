using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Datadog.CustomActions.Rollback;
using WixToolset.Dtf.WindowsInstaller;
using Microsoft.Win32;
using System;
using System.Collections.Generic;
using System.Linq;
using System.Security.AccessControl;
using System.Security.Principal;
using System.ServiceProcess;
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

        public static bool IsServiceDoesNotExistError(Exception exception)
        {
            // Walk the exception chain to find a Win32Exception with ERROR_SERVICE_DOES_NOT_EXIST (1060)
            var current = exception;
            while (current != null)
            {
                if (current is System.ComponentModel.Win32Exception wex && wex.NativeErrorCode == 1060)
                {
                    return true;
                }
                // Fallback: some ServiceController operations surface as InvalidOperationException with a descriptive message
                // System.ComponentModel.Win32Exception: The specified service does not exist as an installed service
                if (current is InvalidOperationException ioe &&
                    ioe.Message != null &&
                    // string.Contains doesn't have a ignore-case option
                    ioe.Message.IndexOf("does not exist", StringComparison.InvariantCultureIgnoreCase) >= 0)
                {
                    return true;
                }
                current = current.InnerException;
            }
            return false;
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

        public static ActionResult EnsureNpmServiceDependency(Session session)
        {
            return EnsureNpmServiceDependendency(new SessionWrapper(session));
        }
        // The installed account is authoritative, as it is during normal installation.
        // Preserve services when its credentials are unavailable for recreation.
        internal ActionResult PrepareServiceRemoval()
        {
            _session["CanDeleteServices"] = "";
            _session["ServiceRemovalAccount"] = "";
            _session["ServiceRemovalSid"] = "";
            try
            {
                using var installed = _registryServices.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey);
                var domain = installed?.GetValue("installedDomain") as string;
                var user = installed?.GetValue("installedUser") as string;
                if (string.IsNullOrEmpty(domain) || string.IsNullOrEmpty(user))
                {
                    _session.Log($"Preserving services: installed Agent account metadata is missing (registry key present: {installed is not null}, domain present: {!string.IsNullOrEmpty(domain)}, user present: {!string.IsNullOrEmpty(user)})");
                    return ActionResult.Success;
                }
                var account = $"{domain}\\{user}";
                if (!_nativeMethods.LookupAccountName(account, out _, out _, out var accountSid, out _))
                {
                    _session.Log($"Preserving services: could not resolve installed Agent account '{account}' to a SID");
                    return ActionResult.Success;
                }
                // Passwords stay in LSA. Only capture the installed account identity for rollback, and
                // preserve registrations if the current password cannot be associated
                // with the installed account (including legacy secrets without a SID).
                if (AgentCredentials.RequiresPassword(_nativeMethods, accountSid)
                    && string.IsNullOrEmpty(AgentCredentials.ReadPassword(_nativeMethods, accountSid)))
                {
                    _session.Log("Preserving services: the installed Agent user password is unavailable or unassociated");
                    return ActionResult.Success;
                }
                _session["ServiceRemovalAccount"] = account;
                _session["ServiceRemovalSid"] = accountSid.Value;
                _session["CanDeleteServices"] = "1";
                _session.Log("Allowing service deletion during upgrade removal: recreation credentials are available");
            }
            catch (Exception e)
            {
                // Credential lookup must not turn a previously supported upgrade into a failure.
                _session.Log($"Preserving services: could not prepare recreation credentials ({e.GetType().Name})");
            }
            return ActionResult.Success;
        }

        public static ActionResult PrepareServiceRemoval(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session)).PrepareServiceRemoval();
        }

        internal ActionResult RestoreServiceRemovalCredentials()
        {
            var account = _session.Property("ServiceRemovalAccount");
            if (string.IsNullOrEmpty(account))
            {
                return ActionResult.Success;
            }
            string password = null;
            try
            {
                var sid = new SecurityIdentifier(_session.Property("ServiceRemovalSid"));
                if (!_nativeMethods.LookupAccountName(account, out _, out _, out var currentSid, out _) || !sid.Equals(currentSid))
                {
                    throw new InvalidOperationException("The outgoing account identity has changed");
                }
                if (AgentCredentials.RequiresPassword(_nativeMethods, sid))
                {
                    password = AgentCredentials.ReadPassword(_nativeMethods, sid);
                    if (string.IsNullOrEmpty(password))
                    {
                        throw new InvalidOperationException("No current password associated with the outgoing account");
                    }
                }
                AgentCredentials.NormalizeServiceCredentials(!AgentCredentials.RequiresPassword(_nativeMethods, sid), sid, ref account, ref password);
            }
            catch (Exception e)
            {
                _session.Log($"Could not restore service credentials: current credentials unavailable for outgoing account; repair may be required ({e.GetType().Name})");
                return ActionResult.Success;
            }
            try
            {
                // Restore the MSI-managed accounts, including LocalSystem services.
                // Do not preserve manually customized service logon accounts.
                SetServiceUsers(account, password, rollback: true);
            }
            catch (Exception e)
            {
                _session.Log($"Failed to restore service credentials ({e.GetType().Name})");
            }
            return ActionResult.Success;
        }

        public static ActionResult RestoreServiceRemovalCredentials(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session)).RestoreServiceRemovalCredentials();
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

        internal void ConfigureServiceUsers(string ddAgentUserName, SecurityIdentifier ddAgentUserSID)
        {
            var ddAgentUserPassword = _session.Property("DDAGENTUSER_PROCESSED_PASSWORD");
            var isServiceAccount = _nativeMethods.IsServiceAccount(ddAgentUserSID);
            if (!isServiceAccount && string.IsNullOrEmpty(ddAgentUserPassword))
            {
                _session.Log("Password not provided, will not change service user password");
            }
            else if (isServiceAccount)
            {
                _session.Log("Ignoring provided password because account is a service account");
            }
            _session.Log($"Configuring services with account {ddAgentUserName}");
            // Missing domain passwords preserve existing service credentials; built-in
            // and managed accounts use the empty/null forms required by ChangeServiceConfig.
            AgentCredentials.NormalizeServiceCredentials(isServiceAccount, ddAgentUserSID,
                ref ddAgentUserName, ref ddAgentUserPassword);
            SetServiceUsers(ddAgentUserName, ddAgentUserPassword);
        }

        private void SetServiceUsers(string ddAgentUserName, string ddAgentUserPassword, bool rollback = false)
        {
            void SetCredentials(string serviceName, string account, string password)
            {
                try
                {
                    _serviceController.SetCredentials(serviceName, account, password);
                }
                catch (Exception e) when (rollback)
                {
                    // Continue restoring other services if one restoration fails.
                    _session.Log($"Failed to restore credentials for {serviceName} ({e.GetType().Name})");
                }
            }

            SetCredentials(Constants.AgentServiceName, ddAgentUserName, ddAgentUserPassword);
            SetCredentials(Constants.TraceAgentServiceName, ddAgentUserName, ddAgentUserPassword);
            if (_serviceController.ServiceExists(Constants.PrivateActionRunnerServiceName))
            {
                SetCredentials(Constants.PrivateActionRunnerServiceName, ddAgentUserName, ddAgentUserPassword);
            }
            // SYSTEM
            // LocalSystem is a SCM specific shorthand that doesn't need to be localized
            SetCredentials(Constants.SystemProbeServiceName, "LocalSystem", "");
            SetCredentials(Constants.ProcessAgentServiceName, "LocalSystem", "");
            SetCredentials(Constants.ProcmgrServiceName, "LocalSystem", "");
            if (!rollback)
            {
                EnableProcmgrService();
            }
            SetCredentials(Constants.InstallerServiceName, "LocalSystem", "");

            SetCredentials(Constants.SecurityAgentServiceName, ddAgentUserName, ddAgentUserPassword);
        }

        private void EnableProcmgrService()
        {
            try
            {
                _session.Log($"Setting {Constants.ProcmgrServiceName} start type to {ServiceStartMode.Manual}");
                _serviceController.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Manual);
            }
            catch (Exception e) when (IsServiceDoesNotExistError(e))
            {
                _session.Log(
                    $"Service {Constants.ProcmgrServiceName} not found, not changing its start type: {e}");
            }
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
                Constants.InstallerServiceName,
                Constants.ProcmgrServiceName,
            };
            if (_serviceController.ServiceExists(Constants.PrivateActionRunnerServiceName))
            {
                services.Add(Constants.PrivateActionRunnerServiceName);
            }

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

        public static ActionResult ConfigureServicesRollback(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session), "ConfigureServices").ConfigureServicesRollback();
        }

        /// <summary>
        /// Stop any existing datadog services
        /// </summary>
        /// <returns></returns>
        public ActionResult StopDDServices(bool continueOnError)
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
                // The driver services must come after every process that opens their devices,
                // otherwise the driver cannot unload and its stop waits out the timeout below.
                // system-probe is one of those processes and dd-procmgr-service may be the one
                // running it, in which case stopping the Windows service by name is a no-op and
                // only stopping the supervisor takes the process down.
                var ddservices = new[]
                {
                    Constants.ProcmgrServiceName,
                    Constants.SystemProbeServiceName,
                    Constants.NpmServiceName,
                    Constants.ProcmonServiceName,       // might not exist depending on compile time options**
                    Constants.SecurityAgentServiceName, // might not exist depending on compile time options**
                    Constants.PrivateActionRunnerServiceName,
                    Constants.ProcessAgentServiceName,
                    Constants.TraceAgentServiceName,
                    Constants.InstallerServiceName,
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
                    catch (Exception e) when (IsServiceDoesNotExistError(e))
                    {
                        _session.Log($"Service {service} not found");
                        continue;
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

        public static ActionResult StopDDServices(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session)).StopDDServices(false);
        }

        private ActionResult StartDDServices()
        {
            // Check if DD_INSTALL_ONLY flag is set
            var installOnly = _session.Property("DD_INSTALL_ONLY");
            if (!string.IsNullOrEmpty(installOnly) && (installOnly == "1" || installOnly.ToLower() == "true"))
            {
                _session.Log("DD_INSTALL_ONLY is set, skipping service start.");
                return ActionResult.Success;
            }

            try
            {
                var ddservices = new[]
                {
                    // only start the main agent service. it should start any other services
                    // that should be running.
                    Constants.AgentServiceName,
                };
                foreach (var service in ddservices)
                {
                    try
                    {
                        using var actionRecord = new Record(
                            "Start Datadog services",
                            $"Starting {service} service",
                            ""
                        );
                        _session.Message(InstallMessage.ActionStart, actionRecord);
                        _serviceController.StartService(service, TimeSpan.FromMinutes(3));
                    }
                    catch (Exception e)
                    {
                        _session.Log($"Failed to start service {service}: {e}");
                        // Allow service start to fail and continue the install
                    }
                }
            }
            catch (Exception e)
            {
                _session.Log($"Failed to start services: {e}");
                // Allow service start to fail and continue the install
            }
            return ActionResult.Success;
        }

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

        public static ActionResult StartDDServicesRollback(Session session)
        {
            return new ServiceCustomAction(new SessionWrapper(session)).StartDDServicesRollback();
        }
    }
}
