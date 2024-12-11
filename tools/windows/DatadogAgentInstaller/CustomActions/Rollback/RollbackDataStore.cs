using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Security.AccessControl;
using System.Security.Principal;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Newtonsoft.Json;
using Newtonsoft.Json.Serialization;

namespace Datadog.CustomActions.Rollback
{
    internal class RollbackDataStore
    {
        private readonly ISession _session;
        private readonly IFileSystemServices _fileSystemServices;
        private readonly IServiceController _serviceController;
        private readonly string _storageName;
        private readonly string _dataPath;

        private List<IRollbackAction> RollbackActions { get; set; }

        static string StorageRootPath() => Path.Combine(Path.GetTempPath(), "datadog-installer", "rollback");

        // Used to safely bind serialized JSON to a .NET type
        // TypeNameHandling.Auto/All is unsafe by itself
        // https://www.newtonsoft.com/json/help/html/SerializeSerializationBinder.htm
        private class KnownTypesBinder : ISerializationBinder
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

        public RollbackDataStore(ISession session, string name, IFileSystemServices fileSystemServices, IServiceController serviceController)
        {
            _session = session;
            _fileSystemServices = fileSystemServices;
            _serviceController = serviceController;
            _storageName = name;
            _dataPath = CreateStoragePath();

            RollbackActions = new List<IRollbackAction>();
        }

        public RollbackDataStore(ISession session, string name)
            : this(
                session,
                name,
                new FileSystemServices(),
                new ServiceController()
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
                // KnownTypes are any IRollbackAction types
                KnownTypes = System.Reflection.Assembly.GetExecutingAssembly().GetTypes()
                    .Where(p => p.GetInterfaces().Contains(typeof(IRollbackAction)) && typeof(IRollbackAction).IsAssignableFrom(p))
                    .ToList()
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
            Load();
            for (var i = RollbackActions.Count - 1; i >= 0; i--)
            {
                RollbackActions[i].Restore(_session, _fileSystemServices, _serviceController);
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
}
