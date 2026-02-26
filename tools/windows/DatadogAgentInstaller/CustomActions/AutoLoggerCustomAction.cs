using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Microsoft.Win32;
using WixToolset.Dtf.WindowsInstaller;
using System;
using System.IO;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Text;

namespace Datadog.CustomActions
{
    public class AutoLoggerCustomAction
    {
        private const string AutoLoggerBasePath = @"SYSTEM\CurrentControlSet\Control\WMI\Autologger";
        private const string SessionName = "Datadog Logon Duration";
        private const string LogonDurationSubDir = "logonduration";
        private const string EtlFileName = "logon_duration.etl";

        private static readonly string SessionKeyPath = $@"{AutoLoggerBasePath}\{SessionName}";

        private static readonly string[] ProviderGuids =
        {
            "{22FB2CD6-0E7B-422B-A0C7-2FAD1FD0E716}", // Microsoft-Windows-Kernel-Process
            "{A68CA8B7-004F-D7B6-A698-07E2DE0F1F5D}", // Microsoft-Windows-Kernel-General
            "{DBE9B383-7CF3-4331-91CC-A3CB16A3B538}", // Microsoft-Windows-Winlogon
            "{89B1E9F0-5AFF-44A6-9B44-0A07A7CE5845}", // Microsoft-Windows-User Profiles Service
            "{AEA1B4FA-97D1-45F2-A64C-4D69FFFD92C9}", // Microsoft-Windows-GroupPolicy
            "{30336ED4-E327-447C-9DE0-51B652C86108}"   // Microsoft-Windows-Shell-Core
        };

        /// <summary>
        /// Resolves the agent user SID. On first install DDAGENTUSER_SID may be empty because
        /// the user is created by the deferred ConfigureUser action after CustomActionData was
        /// captured. In that case, fall back to looking up the SID by DDAGENTUSER_PROCESSED_FQ_NAME.
        /// </summary>
        private static string ResolveAgentUserSid(ISession session)
        {
            var sid = session.Property("DDAGENTUSER_SID");
            if (!string.IsNullOrEmpty(sid))
            {
                return sid;
            }

            var fqName = session.Property("DDAGENTUSER_PROCESSED_FQ_NAME");
            if (string.IsNullOrEmpty(fqName))
            {
                session.Log("Neither DDAGENTUSER_SID nor DDAGENTUSER_PROCESSED_FQ_NAME is set");
                return null;
            }

            try
            {
                session.Log($"DDAGENTUSER_SID not set, resolving SID from account name: {fqName}");
                var account = new NTAccount(fqName);
                var securityIdentifier = (SecurityIdentifier)account.Translate(typeof(SecurityIdentifier));
                session.Log($"Resolved SID: {securityIdentifier.Value}");
                return securityIdentifier.Value;
            }
            catch (Exception e)
            {
                session.Log($"Failed to resolve SID for {fqName}: {e}");
                return null;
            }
        }

        private static ActionResult ConfigureAutoLogger(ISession session)
        {
            try
            {
                var appDataDir = session.Property("APPLICATIONDATADIRECTORY");
                var ddAgentUserSidString = ResolveAgentUserSid(session);

                if (string.IsNullOrEmpty(appDataDir))
                {
                    session.Log("APPLICATIONDATADIRECTORY is not set");
                    return ActionResult.Failure;
                }

                var logonDurationDir = Path.Combine(appDataDir, LogonDurationSubDir);
                var etlFilePath = Path.Combine(logonDurationDir, EtlFileName);

                CreateLogonDurationDirectory(session, logonDurationDir, ddAgentUserSidString);
                CreateAutoLoggerRegistryKeys(session, etlFilePath);

                if (!string.IsNullOrEmpty(ddAgentUserSidString))
                {
                    SetAutoLoggerRegistryPermissions(session, ddAgentUserSidString);
                }
                else
                {
                    session.Log("Could not determine agent user SID, skipping registry permission configuration");
                }

                session.Log("AutoLogger configuration complete");
                return ActionResult.Success;
            }
            catch (Exception e)
            {
                session.Log($"Failed to configure AutoLogger: {e}");
                return ActionResult.Failure;
            }
        }

        private static void CreateLogonDurationDirectory(ISession session, string path, string ddAgentUserSidString)
        {
            session.Log($"Creating logon duration directory: {path}");

            var security = new DirectorySecurity();
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

            if (!string.IsNullOrEmpty(ddAgentUserSidString))
            {
                var ddAgentUserSid = new SecurityIdentifier(ddAgentUserSidString);
                security.AddAccessRule(new FileSystemAccessRule(
                    ddAgentUserSid,
                    FileSystemRights.ReadAndExecute,
                    InheritanceFlags.ObjectInherit | InheritanceFlags.ContainerInherit,
                    PropagationFlags.None,
                    AccessControlType.Allow));
            }

            Directory.CreateDirectory(path, security);
            Directory.SetAccessControl(path, security);
            session.Log($"Logon duration directory created/updated: {path}");
        }

        private static void CreateAutoLoggerRegistryKeys(ISession session, string etlFilePath)
        {
            session.Log($"Creating AutoLogger session key: {SessionKeyPath}");

            using (var sessionKey = Registry.LocalMachine.CreateSubKey(SessionKeyPath))
            {
                if (sessionKey == null)
                {
                    throw new Exception($"Failed to create registry key: {SessionKeyPath}");
                }

                sessionKey.SetValue("Start", 0, RegistryValueKind.DWord);
                sessionKey.SetValue("Guid", GenerateUuidV5(SessionName), RegistryValueKind.String);
                sessionKey.SetValue("BufferSize", 128, RegistryValueKind.DWord);
                sessionKey.SetValue("MaximumBuffers", 32, RegistryValueKind.DWord);
                sessionKey.SetValue("MaxFileSize", 256, RegistryValueKind.DWord);
                sessionKey.SetValue("FileName", etlFilePath, RegistryValueKind.String);
                sessionKey.SetValue("LogFileMode", 1, RegistryValueKind.DWord);

                session.Log("AutoLogger session values configured");
            }

            foreach (var providerGuid in ProviderGuids)
            {
                var providerKeyPath = $@"{SessionKeyPath}\{providerGuid}";
                session.Log($"Creating provider sub-key: {providerKeyPath}");

                using (var providerKey = Registry.LocalMachine.CreateSubKey(providerKeyPath))
                {
                    if (providerKey == null)
                    {
                        throw new Exception($"Failed to create registry key: {providerKeyPath}");
                    }

                    providerKey.SetValue("Enabled", 1, RegistryValueKind.DWord);
                    providerKey.SetValue("EnableLevel", 5, RegistryValueKind.DWord);
                }
            }

            session.Log("AutoLogger provider sub-keys configured");
        }

        private static void SetAutoLoggerRegistryPermissions(ISession session, string ddAgentUserSidString)
        {
            session.Log($"Setting AutoLogger registry permissions for SID: {ddAgentUserSidString}");

            using (var sessionKey = Registry.LocalMachine.OpenSubKey(
                SessionKeyPath,
                RegistryKeyPermissionCheck.ReadWriteSubTree,
                RegistryRights.ChangePermissions | RegistryRights.ReadKey))
            {
                if (sessionKey == null)
                {
                    throw new Exception($"Failed to open registry key for permissions: {SessionKeyPath}");
                }

                var ddAgentUserSid = new SecurityIdentifier(ddAgentUserSidString);
                var registrySecurity = sessionKey.GetAccessControl();

                registrySecurity.AddAccessRule(new RegistryAccessRule(
                    ddAgentUserSid,
                    RegistryRights.SetValue | RegistryRights.QueryValues,
                    AccessControlType.Allow));

                sessionKey.SetAccessControl(registrySecurity);
            }

            session.Log("AutoLogger registry permissions set");
        }

        private static ActionResult RemoveAutoLogger(ISession session)
        {
            var appDataDir = session.Property("APPLICATIONDATADIRECTORY");

            try
            {
                session.Log($"Deleting AutoLogger registry key tree: {SessionKeyPath}");
                Registry.LocalMachine.DeleteSubKeyTree(SessionKeyPath, false);
                session.Log("AutoLogger registry keys removed");
            }
            catch (Exception e)
            {
                session.Log($"Warning: could not remove AutoLogger registry keys: {e}");
            }

            return ActionResult.Success;
        }

        /// <summary>
        /// Generates a deterministic UUID v5 (RFC 4122) from a name using the DNS namespace.
        /// </summary>
        private static string GenerateUuidV5(string name)
        {
            // DNS namespace UUID: 6ba7b810-9dad-11d1-80b4-00c04fd430c8
            var namespaceBytes = new byte[]
            {
                0x6b, 0xa7, 0xb8, 0x10,
                0x9d, 0xad,
                0x11, 0xd1,
                0x80, 0xb4,
                0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8
            };

            var nameBytes = Encoding.UTF8.GetBytes(name);
            byte[] hash;
            using (var sha1 = SHA1.Create())
            {
                sha1.TransformBlock(namespaceBytes, 0, namespaceBytes.Length, null, 0);
                sha1.TransformFinalBlock(nameBytes, 0, nameBytes.Length);
                hash = sha1.Hash;
            }

            // Set version 5 (bits 4-7 of byte 6)
            hash[6] = (byte)((hash[6] & 0x0F) | 0x50);
            // Set variant (bits 6-7 of byte 8)
            hash[8] = (byte)((hash[8] & 0x3F) | 0x80);

            var guid = new Guid(
                (int)(((uint)hash[0] << 24) | ((uint)hash[1] << 16) | ((uint)hash[2] << 8) | hash[3]),
                (short)(((ushort)hash[4] << 8) | hash[5]),
                (short)(((ushort)hash[6] << 8) | hash[7]),
                hash[8], hash[9], hash[10], hash[11],
                hash[12], hash[13], hash[14], hash[15]);

            return guid.ToString("B").ToUpperInvariant();
        }

        public static ActionResult ConfigureAutoLogger(Session session)
        {
            return ConfigureAutoLogger(new SessionWrapper(session));
        }

        public static ActionResult RemoveAutoLogger(Session session)
        {
            return RemoveAutoLogger(new SessionWrapper(session));
        }
    }
}
