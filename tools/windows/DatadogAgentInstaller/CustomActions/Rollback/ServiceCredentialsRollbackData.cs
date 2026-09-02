using System;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Newtonsoft.Json;

namespace Datadog.CustomActions.Rollback
{
    /// <summary>
    /// Snapshots service credentials for ConfigureServices rollback.
    /// Passwords go in LSA, not the rollback JSON, which is written under %TEMP% and logged.
    /// </summary>
    class ServiceCredentialsRollbackData : IRollbackAction
    {
        [JsonProperty("ServiceName")] private string _serviceName;
        [JsonProperty("AccountName")] private string _accountName;
        [JsonProperty("UseEmptyPassword")] private bool _useEmptyPassword;
        [JsonProperty("SecretKey")] private string _secretKey;

        [JsonIgnore] private readonly INativeMethods _nativeMethods;

        [JsonConstructor]
        public ServiceCredentialsRollbackData()
        {
        }

        internal ServiceCredentialsRollbackData(
            string serviceName,
            string accountName,
            bool useEmptyPassword,
            string secretKey,
            INativeMethods nativeMethods)
        {
            _serviceName = serviceName;
            _accountName = accountName;
            _useEmptyPassword = useEmptyPassword;
            _secretKey = secretKey;
            _nativeMethods = nativeMethods;
        }

        internal static string SecretKey(string serviceName)
        {
            // L$ is local-only (Administrators can read; not available remotely).
            return $"L$datadog_installer_rollback_scm_{serviceName}";
        }

        internal static bool IsWellKnownServiceAccount(string accountName)
        {
            return accountName != null &&
                   (accountName.Equals("LocalSystem", StringComparison.OrdinalIgnoreCase) ||
                    accountName.Equals(@"NT AUTHORITY\SYSTEM", StringComparison.OrdinalIgnoreCase) ||
                    accountName.Equals("LocalService", StringComparison.OrdinalIgnoreCase) ||
                    accountName.Equals(@"NT AUTHORITY\LocalService", StringComparison.OrdinalIgnoreCase) ||
                    accountName.Equals("NetworkService", StringComparison.OrdinalIgnoreCase) ||
                    accountName.Equals(@"NT AUTHORITY\NetworkService", StringComparison.OrdinalIgnoreCase));
        }

        internal static ServiceCredentialsRollbackData Capture(
            string serviceName,
            IServiceController serviceController,
            INativeMethods nativeMethods,
            ISession session)
        {
            var accountName = serviceController.GetServiceStartName(serviceName);
            if (string.IsNullOrEmpty(accountName))
            {
                session.Log($"No account name for {serviceName}, skipping credential snapshot");
                return null;
            }

            if (IsWellKnownServiceAccount(accountName))
            {
                session.Log($"Snapshot {serviceName} credentials: account={accountName} password=empty");
                return new ServiceCredentialsRollbackData(
                    serviceName, accountName, useEmptyPassword: true, secretKey: null, nativeMethods);
            }

            string password;
            try
            {
                password = nativeMethods.FetchScmServicePassword(serviceName);
            }
            catch (Exception e)
            {
                session.Log($"Could not read SCM password for {serviceName}, skipping credential snapshot: {e}");
                return null;
            }

            if (string.IsNullOrEmpty(password))
            {
                session.Log($"SCM password for {serviceName} is unavailable, skipping credential snapshot");
                return null;
            }

            var secretKey = SecretKey(serviceName);
            try
            {
                nativeMethods.StoreSecret(secretKey, password);
            }
            catch (Exception e)
            {
                session.Log($"Could not store rollback password for {serviceName}, skipping credential snapshot: {e}");
                return null;
            }

            session.Log($"Snapshot {serviceName} credentials: account={accountName} password=secret");
            return new ServiceCredentialsRollbackData(
                serviceName, accountName, useEmptyPassword: false, secretKey: secretKey, nativeMethods);
        }

        internal static void DiscardSecret(INativeMethods nativeMethods, ISession session, string serviceName)
        {
            try
            {
                nativeMethods.RemoveSecret(SecretKey(serviceName));
            }
            catch (Exception e)
            {
                session.Log($"Could not remove rollback password for {serviceName}: {e}");
            }
        }

        public void Restore(ISession session, IFileSystemServices _, IServiceController serviceController)
        {
            var nativeMethods = _nativeMethods ?? new Win32NativeMethods();
            if (RestoreCredentials(session, serviceController, nativeMethods) && !_useEmptyPassword)
            {
                DiscardSecret(nativeMethods, session, _serviceName);
            }
        }

        private bool RestoreCredentials(
            ISession session,
            IServiceController serviceController,
            INativeMethods nativeMethods)
        {
            string password;
            if (_useEmptyPassword)
            {
                password = "";
            }
            else
            {
                try
                {
                    password = nativeMethods.FetchSecret(_secretKey);
                }
                catch (Exception e)
                {
                    session.Log(
                        $"Could not read rollback password for {_serviceName}, " +
                        $"leaving credentials unchanged: {e}");
                    return false;
                }

                if (string.IsNullOrEmpty(password))
                {
                    session.Log(
                        $"Rollback password for {_serviceName} is empty, leaving credentials unchanged");
                    return false;
                }
            }

            session.Log($"Restoring {_serviceName} service account to {_accountName}");
            try
            {
                serviceController.SetCredentials(_serviceName, _accountName, password);
                return true;
            }
            catch (Exception e)
            {
                session.Log($"Error restoring {_serviceName} credentials: {e}");
                return false;
            }
        }
    }
}
