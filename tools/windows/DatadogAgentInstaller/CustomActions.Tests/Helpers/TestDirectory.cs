using System;
using System.Diagnostics;
using System.IO;
using System.Security.AccessControl;
using System.Security.Principal;
using Datadog.CustomActions.Native;
using FluentAssertions;

namespace CustomActions.Tests.Helpers
{
    /// <summary>
    /// Helpers to create directories with a chosen owner, for the tests that cover the permissions
    /// the custom actions apply.
    /// </summary>
    /// <remarks>
    /// The owner is always set explicitly: a directory created by an elevated process is owned by
    /// Administrators, but by the user account under the identity the CI runs as. Same as
    /// createDirectoryOwnedByUntrusted in pkg/fleet/installer/paths/paths_windows_test.go.
    /// </remarks>
    internal static class TestDirectory
    {
        internal static readonly SecurityIdentifier LocalSystem =
            new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null);

        internal static readonly SecurityIdentifier Administrators =
            new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null);

        /// <summary>
        /// SID of the ContainerAdministrator account used in Windows containers. Not a
        /// WellKnownSidType, so it is constructed from its literal SID string.
        /// </summary>
        internal static readonly SecurityIdentifier ContainerAdministrator =
            new SecurityIdentifier("S-1-5-93-2-1");

        internal static readonly SecurityIdentifier Everyone =
            new SecurityIdentifier(WellKnownSidType.WorldSid, null);

        /// <summary>
        /// An owner that is neither SYSTEM nor Administrators. Guests always exists and is never
        /// trusted, so no account has to be created for the tests.
        /// </summary>
        internal static readonly SecurityIdentifier UntrustedOwner =
            new SecurityIdentifier(WellKnownSidType.BuiltinGuestsSid, null);

        /// <summary>
        /// Create a directory owned by @owner. When @grantEveryone is set the directory also grants
        /// access to everyone and keeps the permissions it inherits.
        /// </summary>
        internal static string CreateOwnedBy(string path, SecurityIdentifier owner, bool grantEveryone = false)
        {
            Directory.CreateDirectory(path);

            if (grantEveryone)
            {
                // Keeping the inherited permissions leaves the WRITE_OWNER access needed to hand the
                // directory to another owner, and to clean it up.
                var security = Directory.GetAccessControl(path, AccessControlSections.Access);
                security.SetAccessRuleProtection(false, true);
                security.AddAccessRule(new FileSystemAccessRule(
                    Everyone,
                    FileSystemRights.Modify,
                    InheritanceFlags.ObjectInherit | InheritanceFlags.ContainerInherit,
                    PropagationFlags.None,
                    AccessControlType.Allow));
                Directory.SetAccessControl(path, security);
            }

            SetOwner(path, owner);
            OwnerOf(path).Should().Be(owner);

            return path;
        }

        internal static void SetOwner(string path, SecurityIdentifier owner)
        {
            EnableSetOwnerPrivilege();

            var security = new DirectorySecurity();
            security.SetOwner(owner);
            Directory.SetAccessControl(path, security);
        }

        internal static SecurityIdentifier OwnerOf(string path)
        {
            return (SecurityIdentifier)Directory.GetAccessControl(path, AccessControlSections.Owner)
                .GetOwner(typeof(SecurityIdentifier));
        }

        internal static void CreateJunction(string path, string target)
        {
            Run($"mklink /J \"{path}\" \"{target}\"");
            ((File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0).Should().BeTrue();
        }

        /// <summary>
        /// Take ownership of @path and grant access to the identity running the tests, so a directory
        /// the tests made inaccessible can be deleted.
        /// </summary>
        internal static void ReclaimForCleanup(string path)
        {
            var current = WindowsIdentity.GetCurrent().User;
            SetOwner(path, current);

            var security = Directory.GetAccessControl(path, AccessControlSections.Access);
            security.AddAccessRule(new FileSystemAccessRule(
                current,
                FileSystemRights.FullControl,
                InheritanceFlags.ObjectInherit | InheritanceFlags.ContainerInherit,
                PropagationFlags.None,
                AccessControlType.Allow));
            Directory.SetAccessControl(path, security);
        }

        /// <summary>
        /// Enable the privilege needed to make SYSTEM the owner of a directory.
        /// </summary>
        internal static void EnableSetOwnerPrivilege()
        {
            new Win32NativeMethods().EnablePrivilege("SeRestorePrivilege");
        }

        /// <summary>
        /// Make the directories the rollback data store uses owned by Administrators.
        /// </summary>
        internal static void PrepareRollbackStore()
        {
            lock (RollbackStoreLock)
            {
                if (_rollbackStorePrepared)
                {
                    return;
                }

                var storeBase = Path.Combine(Path.GetTempPath(), "datadog-installer");
                foreach (var path in new[] { storeBase, Path.Combine(storeBase, "rollback") })
                {
                    Directory.CreateDirectory(path);
                    SetOwner(path, Administrators);
                }

                _rollbackStorePrepared = true;
            }
        }

        private static readonly object RollbackStoreLock = new object();
        private static bool _rollbackStorePrepared;

        private static void Run(string command)
        {
            var process = System.Diagnostics.Process.Start(
                new ProcessStartInfo("cmd.exe", $"/c {command}")
                {
                    UseShellExecute = false,
                    CreateNoWindow = true,
                    RedirectStandardOutput = true,
                    RedirectStandardError = true
                });
            var output = process.StandardOutput.ReadToEnd() + process.StandardError.ReadToEnd();
            process.WaitForExit();
            process.ExitCode.Should().Be(0, $"\"{command}\" failed: {output}");
        }
    }
}
