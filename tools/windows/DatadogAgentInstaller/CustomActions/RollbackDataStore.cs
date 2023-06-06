using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Text;
using System.Threading.Tasks;
using Microsoft.Win32;
using Newtonsoft.Json;
using Newtonsoft.Json.Serialization;

namespace Datadog.CustomActions
{
    internal class RollbackDataStore
    {
        private readonly ISession _session;
        private readonly IFileSystemServices _fileSystemServices;
        private readonly string _storageName;
        private readonly string _dataPath;

        private List<IRollbackAction> RollbackActions;

        static string StorageRootPath() => Path.Combine(Path.GetTempPath(), "datadog-installer", "rollback");

        public RollbackDataStore(ISession session, string name, IFileSystemServices fileSystemServices)
        {
            _session = session;
            _fileSystemServices = fileSystemServices;
            _storageName = name;
            _dataPath = CreateStoragePath();

            RollbackActions = new List<IRollbackAction>();
        }

        public RollbackDataStore(ISession session, string name)
            : this(
                session,
                name,
                new FileSystemServices()
            )
        {
        }

        /// <summary>
        /// Create, configure, and return the path to be used to store this rollback data
        /// </summary>
        private string CreateStoragePath()
        {
            var parent = StorageRootPath();
            var path = Path.Combine(parent, $"{_storageName}.json");

            CreateDirectory(parent);

            return path;
        }

        /// <summary>
        /// Create directory @path and secure it so only SYSTEM and Administrators have access
        /// </summary>
        /// <param name="path"></param>
        private void CreateDirectory(string path)
        {
            // Create DACL for only SYSTEM and Administrators, disable inheritance
            FileSystemSecurity security = new DirectorySecurity();
            security.SetAccessRuleProtection(true, false);
            security.AddAccessRule(new FileSystemAccessRule(
                new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null),
                FileSystemRights.FullControl,
                InheritanceFlags.ObjectInherit | InheritanceFlags.ContainerInherit,
                PropagationFlags.None,
                AccessControlType.Allow));
            security.AddAccessRule(new FileSystemAccessRule(
                new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null),
                FileSystemRights.FullControl,
                InheritanceFlags.ObjectInherit | InheritanceFlags.ContainerInherit,
                PropagationFlags.None,
                AccessControlType.Allow));

            // Create the directory with the DACL
            Directory.CreateDirectory(path, (DirectorySecurity)security);
        }

        /// <summary>
        /// Return settings to be used for serializing/deserializing the rollback data
        /// To avoid deserialization vulnerabilities, use a SerializationBinder to restrict the
        /// deserializable types.
        /// </summary>
        private JsonSerializerSettings GetSerializerSettings() => new()
        {
            TypeNameHandling = TypeNameHandling.Objects,
            SerializationBinder = new KnownTypesBinder
            {
                KnownTypes = new List<Type>
                {
                    typeof(FilePermissionRollbackInfo)
                }
            }
        };

        /// <summary>
        /// Add an action the the store of rollback actions
        /// </summary>
        /// <param name="action"></param>
        public void Add(IRollbackAction action)
        {
            RollbackActions.Add(action);
        }

        /// <summary>
        /// Apply the rollback actions.
        /// Actions are applied in reverse order as they were added to the store.
        /// </summary>
        public void Restore()
        {
            for (int i = RollbackActions.Count - 1; i >= 0; i--)
            {
                RollbackActions[i].Restore(_session, _fileSystemServices);
            }
        }

        /// <summary>
        /// Read and deserialize the rollback actions from the file store
        /// </summary>
        public void Load()
        {
            var jsonString = File.ReadAllText(_dataPath);
            _session.Log($"Loading rollback info: {jsonString}");
            RollbackActions = JsonConvert.DeserializeObject<List<IRollbackAction>>(jsonString, GetSerializerSettings());
        }

        /// <summary>
        /// Serialize and write the rollback actions to the file store
        /// </summary>
        public void Store()
        {
            var jsonString = JsonConvert.SerializeObject(RollbackActions, Formatting.Indented, GetSerializerSettings());
            _session.Log($"Saving rollback info: {jsonString}");
            File.WriteAllText(_dataPath, jsonString);
        }
    }

    // Used to safely bind serialized JSON to a .NET type
    // TypeNameHandling.Auto/All is unsafe by itself
    // https://www.newtonsoft.com/json/help/html/SerializeSerializationBinder.htm
    public class KnownTypesBinder : ISerializationBinder
    {
        public IList<Type> KnownTypes { get; set; }

        public Type BindToType(string assemblyName, string typeName)
        {
            return KnownTypes.SingleOrDefault(t => t.Name == typeName);
        }

        public void BindToName(Type serializedType, out string assemblyName, out string typeName)
        {
            assemblyName = null;
            typeName = serializedType.Name;
        }
    }

    interface IRollbackAction
    {
        public void Restore(ISession session, IFileSystemServices fileSystemServices);
    }

    class FilePermissionRollbackInfo : IRollbackAction
    {
        [JsonProperty("FilePath")] private string FilePath;
        [JsonProperty("SDDL")] private string SDDL;

        [JsonConstructor]
        public FilePermissionRollbackInfo()
        {
        }

        public FilePermissionRollbackInfo(string filePath, IFileSystemServices fileSystemServices)
        {
            var fileSystemSecurity = fileSystemServices.GetAccessControl(filePath, AccessControlSections.All);
            SDDL = fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All);
            FilePath = filePath;
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
        public void Restore(ISession session, IFileSystemServices fileSystemServices)
        {
            FileSystemSecurity fileSystemSecurity = fileSystemServices.GetAccessControl(FilePath);
            session.Log(
                $"{FilePath} current ACLs: {fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");
            fileSystemSecurity.SetSecurityDescriptorSddlForm(SDDL);
            session.Log($"{FilePath} rollback SDDL {SDDL}");
            try
            {
                fileSystemServices.SetAccessControl(FilePath, fileSystemSecurity);
            }
            catch (Exception e)
            {
                session.Log($"Error writing ACL: {e}");
            }

            fileSystemSecurity = fileSystemServices.GetAccessControl(FilePath);
            session.Log(
                $"{FilePath} new ACLs: {fileSystemSecurity.GetSecurityDescriptorSddlForm(AccessControlSections.All)}");
        }
    }
}
