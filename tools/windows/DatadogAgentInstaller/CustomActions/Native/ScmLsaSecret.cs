using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;
using Datadog.CustomActions.Interfaces;
using Microsoft.Win32;

namespace Datadog.CustomActions.Native
{
    internal static class ScmLsaSecret
    {
        private const string LsaSecretsKey = @"SECURITY\Policy\Secrets";
        private const string ScmSecretPrefix = "_SC_";
        private const int ErrorSuccess = 0;
        private const int ErrorNoMoreItems = 259;
        private const int KeyAllAccess = 0xF003F;
        private static readonly IntPtr HkeyLocalMachine = new IntPtr(unchecked((int)0x80000002));

        public static string ReadServicePassword(INativeMethods nativeMethods, string serviceName)
        {
            if (string.IsNullOrEmpty(serviceName))
            {
                return null;
            }

            TryEnablePrivilege(nativeMethods, "SeBackupPrivilege");
            TryEnablePrivilege(nativeMethods, "SeRestorePrivilege");

            var sourceSubkey = $@"{LsaSecretsKey}\{ScmSecretPrefix}{serviceName}";
            var tempLeaf = $"datadoginstaller{Guid.NewGuid():N}";
            var tempSubkey = $@"{LsaSecretsKey}\{tempLeaf}";

            var sourceKey = IntPtr.Zero;
            var destinationKey = IntPtr.Zero;
            try
            {
                if (RegOpenKeyEx(HkeyLocalMachine, sourceSubkey, 0, KeyAllAccess, out sourceKey) != ErrorSuccess)
                {
                    return null;
                }

                if (RegCreateKeyEx(
                        HkeyLocalMachine,
                        tempSubkey,
                        0,
                        null,
                        0,
                        KeyAllAccess,
                        IntPtr.Zero,
                        out destinationKey,
                        out _) != ErrorSuccess)
                {
                    throw new Win32Exception(Marshal.GetLastWin32Error(),
                        $"RegCreateKeyEx({tempSubkey}) failed");
                }

                CopySecretChildren(sourceKey, destinationKey);
                return nativeMethods.FetchSecret(tempLeaf);
            }
            finally
            {
                if (destinationKey != IntPtr.Zero)
                {
                    RegCloseKey(destinationKey);
                }

                if (sourceKey != IntPtr.Zero)
                {
                    RegCloseKey(sourceKey);
                }

                try
                {
                    Registry.LocalMachine.DeleteSubKeyTree(tempSubkey);
                }
                catch
                {
                }
            }
        }

        private static void TryEnablePrivilege(INativeMethods nativeMethods, string privilegeName)
        {
            try
            {
                nativeMethods.EnablePrivilege(privilegeName);
            }
            catch (Exception)
            {
            }
        }

        private static void CopySecretChildren(IntPtr sourceKey, IntPtr destinationKey)
        {
            var index = 0u;
            while (true)
            {
                var nameBuilder = new StringBuilder(256);
                var nameLength = (uint)nameBuilder.Capacity;
                var status = RegEnumKeyEx(
                    sourceKey,
                    index,
                    nameBuilder,
                    ref nameLength,
                    IntPtr.Zero,
                    null,
                    IntPtr.Zero,
                    IntPtr.Zero);
                if (status == ErrorNoMoreItems)
                {
                    break;
                }

                if (status != ErrorSuccess)
                {
                    throw new Win32Exception(status, "RegEnumKeyEx failed while copying SCM LSA secret children");
                }

                var childName = nameBuilder.ToString();
                if (RegOpenKeyEx(sourceKey, childName, 0, KeyAllAccess, out var sourceChild) != ErrorSuccess)
                {
                    throw new Win32Exception(Marshal.GetLastWin32Error(),
                        $"RegOpenKeyEx({childName}) failed while copying SCM LSA secret children");
                }

                try
                {
                    if (RegCreateKeyEx(
                            destinationKey,
                            childName,
                            0,
                            null,
                            0,
                            KeyAllAccess,
                            IntPtr.Zero,
                            out var destinationChild,
                            out _) != ErrorSuccess)
                    {
                        throw new Win32Exception(Marshal.GetLastWin32Error(),
                            $"RegCreateKeyEx({childName}) failed while copying SCM LSA secret children");
                    }

                    try
                    {
                        status = RegCopyTree(sourceChild, null, destinationChild);
                        if (status != ErrorSuccess)
                        {
                            throw new Win32Exception(status,
                                $"RegCopyTree failed while copying SCM LSA secret child {childName}");
                        }
                    }
                    finally
                    {
                        RegCloseKey(destinationChild);
                    }
                }
                finally
                {
                    RegCloseKey(sourceChild);
                }

                index++;
            }
        }

        [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern int RegOpenKeyEx(
            IntPtr hKey,
            string lpSubKey,
            uint ulOptions,
            int samDesired,
            out IntPtr phkResult);

        [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern int RegCreateKeyEx(
            IntPtr hKey,
            string lpSubKey,
            uint Reserved,
            string lpClass,
            uint dwOptions,
            int samDesired,
            IntPtr lpSecurityAttributes,
            out IntPtr phkResult,
            out int lpdwDisposition);

        [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern int RegEnumKeyEx(
            IntPtr hKey,
            uint dwIndex,
            StringBuilder lpName,
            ref uint lpcchName,
            IntPtr lpReserved,
            StringBuilder lpClass,
            IntPtr lpcchClass,
            IntPtr lpftLastWriteTime);

        [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern int RegCopyTree(IntPtr hKeySrc, string lpSubKey, IntPtr hKeyDest);

        [DllImport("advapi32.dll", SetLastError = true)]
        private static extern int RegCloseKey(IntPtr hKey);
    }
}
