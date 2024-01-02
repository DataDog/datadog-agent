using System;
using System.Security.AccessControl;
using Newtonsoft.Json;
using Datadog.CustomActions.Interfaces;
using System.ServiceProcess;

namespace Datadog.CustomActions.RollbackData
{
    class ServicePermissionRollbackData : IRollbackAction
    {
        [JsonProperty("ServiceName")] private string _serviceName;
        [JsonProperty("SDDL")] private string _sddl;

        [JsonConstructor]
        public ServicePermissionRollbackData()
        {
        }

        public ServicePermissionRollbackData(string serviceName, IServiceController serviceController)
        {
            var securityDescriptor = serviceController.GetAccessSecurity(serviceName);
            _sddl = securityDescriptor.GetSddlForm(AccessControlSections.All);
            _serviceName = serviceName;
        }

        /// <summary>
        /// Set service access/object security
        /// https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-setserviceobjectsecurity
        /// </summary>
        public void Restore(ISession session, IFileSystemServices _, IServiceController serviceController)
        {
            var securityDescriptor = serviceController.GetAccessSecurity(_serviceName);
            session.Log(
                $"{_serviceName} current ACLs: {securityDescriptor.GetSddlForm(AccessControlSections.All)}");
            securityDescriptor = new CommonSecurityDescriptor(false, false, _sddl);
            session.Log($"{_serviceName} rollback SDDL {_sddl}");
            try
            {
                serviceController.SetAccessSecurity(_serviceName, securityDescriptor);
            }
            catch (Exception e)
            {
                session.Log($"Error writing ACL: {e}");
            }

            securityDescriptor = serviceController.GetAccessSecurity(_serviceName);
            session.Log(
                $"{_serviceName} new ACLs: {securityDescriptor.GetSddlForm(AccessControlSections.All)}");
        }
    }
}
