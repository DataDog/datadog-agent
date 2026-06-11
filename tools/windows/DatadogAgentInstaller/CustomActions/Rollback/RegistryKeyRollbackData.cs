using System;
using System.Collections.Generic;
using Datadog.CustomActions.Interfaces;
using Microsoft.Win32;
using Newtonsoft.Json;

namespace Datadog.CustomActions.Rollback
{
    /// <summary>
    /// Snapshots a registry key tree (values + sub-keys) and restores it on rollback.
    /// If the key did not exist before, rollback deletes the entire sub-key tree.
    /// If the key existed, rollback restores the original values and sub-keys.
    /// </summary>
    class RegistryKeyRollbackData : IRollbackAction
    {
        [JsonProperty("KeyPath")] private string _keyPath;
        [JsonProperty("ExistedBefore")] private bool _existedBefore;
        [JsonProperty("Values")] private List<RegistryValueSnapshot> _values;
        [JsonProperty("SubKeys")] private List<RegistryKeyRollbackData> _subKeys;

        [JsonConstructor]
        public RegistryKeyRollbackData()
        {
        }

        /// <summary>
        /// Snapshot the registry key tree at <paramref name="keyPath"/> under HKLM.
        /// </summary>
        public RegistryKeyRollbackData(string keyPath)
        {
            _keyPath = keyPath;
            _values = new List<RegistryValueSnapshot>();
            _subKeys = new List<RegistryKeyRollbackData>();

            using (var key = Registry.LocalMachine.OpenSubKey(keyPath))
            {
                _existedBefore = key != null;
                if (key == null)
                {
                    return;
                }

                foreach (var valueName in key.GetValueNames())
                {
                    _values.Add(new RegistryValueSnapshot(
                        valueName,
                        key.GetValue(valueName, null, RegistryValueOptions.DoNotExpandEnvironmentNames),
                        key.GetValueKind(valueName)));
                }

                foreach (var subKeyName in key.GetSubKeyNames())
                {
                    _subKeys.Add(new RegistryKeyRollbackData($@"{keyPath}\{subKeyName}"));
                }
            }
        }

        public void Restore(ISession session, IFileSystemServices fileSystemServices, IServiceController serviceController)
        {
            if (!_existedBefore)
            {
                session.Log($"Registry key {_keyPath} did not exist before; removing");
                try
                {
                    Registry.LocalMachine.DeleteSubKeyTree(_keyPath, false);
                }
                catch (Exception e)
                {
                    session.Log($"Warning: could not remove registry key tree {_keyPath}: {e}");
                }
                return;
            }

            session.Log($"Restoring registry key {_keyPath} to pre-install state");
            try
            {
                Registry.LocalMachine.DeleteSubKeyTree(_keyPath, false);
                RestoreKeyTree(session);
            }
            catch (Exception e)
            {
                session.Log($"Warning: could not restore registry key {_keyPath}: {e}");
            }
        }

        private void RestoreKeyTree(ISession session)
        {
            using (var key = Registry.LocalMachine.CreateSubKey(_keyPath))
            {
                if (key == null)
                {
                    session.Log($"Warning: could not recreate registry key {_keyPath}");
                    return;
                }

                foreach (var value in _values)
                {
                    key.SetValue(value.Name, value.GetTypedData(), value.Kind);
                }
            }

            foreach (var subKey in _subKeys)
            {
                subKey.RestoreKeyTree(session);
            }
        }
    }

    /// <summary>
    /// Stores a single registry value (name, data, kind) for serialization.
    /// </summary>
    class RegistryValueSnapshot
    {
        [JsonProperty("Name")] public string Name { get; private set; }
        [JsonProperty("Data")] public string Data { get; private set; }
        [JsonProperty("Kind")] public RegistryValueKind Kind { get; private set; }

        [JsonConstructor]
        public RegistryValueSnapshot()
        {
        }

        public RegistryValueSnapshot(string name, object data, RegistryValueKind kind)
        {
            Name = name;
            Kind = kind;
            Data = SerializeData(data, kind);
        }

        /// <summary>
        /// Deserialize the stored string back to the appropriate .NET type for SetValue.
        /// </summary>
        public object GetTypedData()
        {
            switch (Kind)
            {
                case RegistryValueKind.DWord:
                    return int.Parse(Data);
                case RegistryValueKind.QWord:
                    return long.Parse(Data);
                case RegistryValueKind.Binary:
                    return Convert.FromBase64String(Data);
                case RegistryValueKind.MultiString:
                    return JsonConvert.DeserializeObject<string[]>(Data);
                case RegistryValueKind.ExpandString:
                case RegistryValueKind.String:
                default:
                    return Data;
            }
        }

        private static string SerializeData(object data, RegistryValueKind kind)
        {
            switch (kind)
            {
                case RegistryValueKind.DWord:
                case RegistryValueKind.QWord:
                    return data.ToString();
                case RegistryValueKind.Binary:
                    return Convert.ToBase64String((byte[])data);
                case RegistryValueKind.MultiString:
                    return JsonConvert.SerializeObject((string[])data);
                case RegistryValueKind.ExpandString:
                case RegistryValueKind.String:
                default:
                    return data?.ToString() ?? "";
            }
        }
    }
}
