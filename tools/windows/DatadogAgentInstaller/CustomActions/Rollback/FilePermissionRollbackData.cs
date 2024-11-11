using System;
using System.Security.AccessControl;
using Datadog.CustomActions.Interfaces;
using Newtonsoft.Json;

namespace Datadog.CustomActions.Rollback
{
    class FilePermissionRollbackData : IRollbackAction
    {
        [JsonProperty("FilePath")] private string _filePath;
        [JsonProperty("SDDL")] private string _sddl;

        [JsonConstructor]
        public FilePermissionRollbackData()
        {
        }

        public FilePermissionRollbackData(string filePath, IFileSystemServices fileSystemServices)
        {
            var fileSystemSecurity = fileSystemServices.GetAccessControl(filePath, AccessControlSections.All);
            _sddl = fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All);
            _filePath = filePath;
        }

        /// <summary>
        /// Write the full @SDDL (Owner, Group, DACL) to @FilePath
        /// If the new Owner/Group are different from the current Owner/Group this operation requires SeRestorePrivilege.
        /// </summary>
        /// <remarks>
        /// If setting the SDDL on a container with an inheritable ACE Windows propagates/updates the inherited ACE on children.
        /// During this, if the SDDL contains owner/group for some reason Windows will also update the owner/group of the children.
        /// The owner/group on children is not changed, but Windows includes the parameter to set access control call. If the owner/group
        /// of that child is different than the current user then inherited ACE propagation for that file will fail unless this function
        /// is called with SeRestorePrivilege enabled. The error is NOT returned by the .NET API, so there's no way to tell that this occurred
        /// until looking at the DACL of the child.
        /// </remarks>
        public void Restore(ISession session, IFileSystemServices fileSystemServices, IServiceController _)
        {
            var fileSystemSecurity = fileSystemServices.GetAccessControl(_filePath);
            session.Log(
                $"{_filePath} current ACLs: {fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");
            fileSystemSecurity.SetSecurityDescriptorSddlForm(_sddl);
            session.Log($"{_filePath} rollback SDDL {_sddl}");
            try
            {
                fileSystemServices.SetAccessControl(_filePath, fileSystemSecurity);
            }
            catch (Exception e)
            {
                session.Log($"Error writing ACL: {e}");
            }

            fileSystemSecurity = fileSystemServices.GetAccessControl(_filePath);
            session.Log(
                $"{_filePath} new ACLs: {fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");
        }
    }
}
