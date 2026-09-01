using System;
using System.ComponentModel;
using System.IO;
using System.Runtime.InteropServices;
using System.Security.AccessControl;
using System.Security.Principal;
using Datadog.CustomActions.Interfaces;
using Microsoft.Win32.SafeHandles;

namespace Datadog.CustomActions.Native
{
    /// <summary>
    /// Creates and validates directories that must only be accessible to SYSTEM and Administrators.
    /// </summary>
    /// <remarks>
    /// Any user can create directories in C:\ProgramData, so a directory may already exist when the
    /// installer runs. A directory not owned by SYSTEM or Administrators is rejected, which fails the
    /// install: an unprivileged user cannot set either as the owner, so the owner is what tells us the
    /// directory was created by a trusted party.
    ///
    /// The Go installer applies the same rules to the directories it creates, see
    /// SecureCreateDirectory/IsDirSecure in pkg/fleet/installer/paths/installer_paths_windows.go.
    /// </remarks>
    internal static class SecureDirectory
    {
        /// <summary>
        /// Create @path granting access to SYSTEM and Administrators only.
        ///
        /// An existing directory must be owned by SYSTEM or Administrators, otherwise a
        /// <see cref="SecureDirectoryException"/> is thrown. Its permissions are reset, which removes
        /// any additional access that had been granted on it.
        /// </summary>
        internal static void CreateAndSecure(ISession session, string path)
        {
            session.Log(Directory.Exists(path) ? $"Securing existing {path}" : $"Creating {path}");

            var security = AdminOnlySecurity();

            // Applies the descriptor at creation, so the directory is never reachable without it, and
            // does nothing at all if it already exists.
            //
            // Only @path is secured. A missing parent is created with inherited permissions and is not
            // checked.
            Directory.CreateDirectory(path, security);

            // The handle does not follow reparse points, so a junction here is the object inspected,
            // never its target.
            SafeFileHandle handle;
            try
            {
                handle = OpenDirectory(path, ReadControl | WriteDac | WriteOwner);
            }
            catch (Win32Exception e)
            {
                throw new SecureDirectoryException(CannotOpenMessage(path, e));
            }

            using (handle)
            {
                var owner = GetOwner(handle);
                if (!IsTrustedOwner(owner))
                {
                    throw new SecureDirectoryException(UntrustedOwnerMessage(path, owner));
                }

                session.Log($"{path} is owned by {Describe(owner)}, resetting its permissions");
                SetSecurity(handle, security);
            }
        }

        /// <summary>
        /// Throw if @path exists and is not owned by SYSTEM or Administrators.
        /// Unlike <see cref="CreateAndSecure"/> this neither creates nor modifies anything, so it can
        /// be used to fail an install before it has made any change to the system.
        /// </summary>
        internal static void AssertSecureOwner(ISession session, string path)
        {
            // Opened rather than tested with Directory.Exists, which also returns false for a path that
            // is a file or cannot be read, and would report those as nothing to verify.
            SafeFileHandle handle;
            try
            {
                handle = OpenDirectory(path, ReadControl);
            }
            catch (Win32Exception e) when (e.NativeErrorCode == ErrorFileNotFound ||
                                           e.NativeErrorCode == ErrorPathNotFound)
            {
                session.Log($"{path} does not exist, nothing to verify");
                return;
            }
            catch (Win32Exception e)
            {
                throw new SecureDirectoryException(CannotOpenMessage(path, e));
            }

            using (handle)
            {
                var owner = GetOwner(handle);
                if (!IsTrustedOwner(owner))
                {
                    throw new SecureDirectoryException(UntrustedOwnerMessage(path, owner));
                }

                session.Log($"{path} is owned by {Describe(owner)}");
            }
        }

        /// <summary>
        /// The message shown to the user when a directory cannot be trusted. It explains how to
        /// resolve the problem, because it stops the installation.
        /// </summary>
        /// <remarks>
        /// Kept short, the dialogs that show it have a fixed size and clip what does not fit.
        /// </remarks>
        private static string UntrustedOwnerMessage(string path, SecurityIdentifier owner)
        {
            return $"{path} has unexpected owner {Describe(owner)}, it must be owned by Administrators " +
                   "or SYSTEM. The installer will not use a directory that a user without administrator " +
                   "rights may have created. Remove it, or make Administrators its owner by running " +
                   $"takeown.exe /A /F {PathForCommand(path)} after reviewing its contents, then retry.";
        }

        /// <summary>
        /// The message shown to the user when the directory cannot be opened to check it.
        /// </summary>
        private static string CannotOpenMessage(string path, Win32Exception error)
        {
            return $"Failed to open {path} to verify its owner: {error.Message} Remove the directory, " +
                   $"or take ownership of it with takeown.exe /A /F {PathForCommand(path)} after " +
                   "verifying its contents, then retry.";
        }

        /// <summary>
        /// Quote @path so that it can be pasted into a command, and remove the trailing separator.
        /// </summary>
        /// <remarks>
        /// The directory properties the installer resolves end with a separator, which would escape the
        /// closing quote. The quotes are needed because a configured directory can contain spaces.
        /// </remarks>
        internal static string PathForCommand(string path)
        {
            return $"\"{path?.TrimEnd('\\', '/')}\"";
        }

        /// <summary>
        /// SYSTEM as owner and group, and a protected DACL (inherits nothing from the parent) granting
        /// full control to SYSTEM and Administrators, inheritable by children.
        ///
        /// Same descriptor as ConfigureUserCustomActions.SetBaseInheritablePermissions.
        /// </summary>
        internal static DirectorySecurity AdminOnlySecurity()
        {
            var localSystem = new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null);

            var security = new DirectorySecurity();
            security.SetAccessRuleProtection(true, false);
            security.AddAccessRule(new FileSystemAccessRule(
                localSystem,
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
            security.SetOwner(localSystem);
            security.SetGroup(localSystem);

            return security;
        }

        /// <summary>
        /// The owners we accept. An unprivileged user cannot set either of them as the owner of a
        /// directory they create.
        /// </summary>
        internal static bool IsTrustedOwner(SecurityIdentifier owner)
        {
            return owner.IsWellKnown(WellKnownSidType.LocalSystemSid) ||
                   owner.IsWellKnown(WellKnownSidType.BuiltinAdministratorsSid);
        }

        /// <summary>
        /// Return the account name for @sid, falling back to its string form when it cannot be
        /// resolved, for example because the account has been deleted.
        /// </summary>
        private static string Describe(SecurityIdentifier sid)
        {
            try
            {
                return $"{sid.Translate(typeof(NTAccount))} ({sid})";
            }
            catch (Exception)
            {
                return sid.ToString();
            }
        }

        #region Native helpers

        // .NET has no handle based equivalent of Directory.Get/SetAccessControl, which take a path.
        // Microsoft's guidance is to call the Win32 functions directly.
        // https://learn.microsoft.com/en-us/dotnet/api/system.io.filesystemaclextensions

        // SE_OBJECT_TYPE.SE_FILE_OBJECT
        private const int SeFileObject = 1;

        private const uint OwnerSecurityInformation = 0x00000001;
        private const uint GroupSecurityInformation = 0x00000002;
        private const uint DaclSecurityInformation = 0x00000004;
        private const uint ProtectedDaclSecurityInformation = 0x80000000;

        private const uint ReadControl = 0x00020000;
        private const uint WriteDac = 0x00040000;
        private const uint WriteOwner = 0x00080000;

        private const uint OpenExisting = 3;
        private const uint FileShareRead = 0x00000001;
        private const uint FileShareWrite = 0x00000002;
        private const uint FileShareDelete = 0x00000004;
        private const uint FileFlagBackupSemantics = 0x02000000;
        private const uint FileFlagOpenReparsePoint = 0x00200000;
        private const uint FileAttributeDirectory = 0x00000010;

        private const int ErrorFileNotFound = 2;
        private const int ErrorPathNotFound = 3;

        [StructLayout(LayoutKind.Sequential)]
        private struct ByHandleFileInformation
        {
            public uint FileAttributes;
            public long CreationTime;
            public long LastAccessTime;
            public long LastWriteTime;
            public uint VolumeSerialNumber;
            public uint FileSizeHigh;
            public uint FileSizeLow;
            public uint NumberOfLinks;
            public uint FileIndexHigh;
            public uint FileIndexLow;
        }

        [DllImport("kernel32.dll")]
        private static extern IntPtr LocalFree(IntPtr mem);

        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern SafeFileHandle CreateFileW(
            string lpFileName,
            uint dwDesiredAccess,
            uint dwShareMode,
            IntPtr lpSecurityAttributes,
            uint dwCreationDisposition,
            uint dwFlagsAndAttributes,
            IntPtr hTemplateFile);

        [DllImport("kernel32.dll", SetLastError = true)]
        private static extern bool GetFileInformationByHandle(SafeFileHandle handle,
            out ByHandleFileInformation information);

        [DllImport("advapi32.dll")]
        private static extern uint GetSecurityInfo(
            SafeFileHandle handle,
            int objectType,
            uint securityInformation,
            out IntPtr owner,
            out IntPtr group,
            out IntPtr dacl,
            out IntPtr sacl,
            out IntPtr securityDescriptor);

        [DllImport("advapi32.dll", SetLastError = true)]
        private static extern bool GetSecurityDescriptorOwner(
            IntPtr securityDescriptor,
            out IntPtr owner,
            out bool ownerDefaulted);

        [DllImport("advapi32.dll", SetLastError = true)]
        private static extern bool GetSecurityDescriptorGroup(
            IntPtr securityDescriptor,
            out IntPtr group,
            out bool groupDefaulted);

        [DllImport("advapi32.dll", SetLastError = true)]
        private static extern bool GetSecurityDescriptorDacl(
            IntPtr securityDescriptor,
            out bool daclPresent,
            out IntPtr dacl,
            out bool daclDefaulted);

        [DllImport("advapi32.dll")]
        private static extern uint SetSecurityInfo(
            SafeFileHandle handle,
            int objectType,
            uint securityInformation,
            IntPtr owner,
            IntPtr group,
            IntPtr dacl,
            IntPtr sacl);

        /// <summary>
        /// Open a handle to the directory itself, with @access. FILE_FLAG_OPEN_REPARSE_POINT means a
        /// junction or symlink at this path is opened as the link, never its target.
        /// </summary>
        /// <exception cref="Win32Exception">
        /// The path could not be opened. The caller decides what the error code means.
        /// </exception>
        /// <exception cref="SecureDirectoryException">The path is not a directory.</exception>
        private static SafeFileHandle OpenDirectory(string path, uint access)
        {
            var handle = CreateFileW(
                path,
                access,
                FileShareRead | FileShareWrite | FileShareDelete,
                IntPtr.Zero,
                OpenExisting,
                FileFlagBackupSemantics | FileFlagOpenReparsePoint,
                IntPtr.Zero);
            if (handle.IsInvalid)
            {
                var error = Marshal.GetLastWin32Error();
                handle.Dispose();
                throw new Win32Exception(error);
            }

            try
            {
                // Reject a file, FILE_FLAG_BACKUP_SEMANTICS opens one just as happily as a directory
                if (!IsDirectory(handle))
                {
                    throw new SecureDirectoryException(
                        $"{path} exists but is not a directory. Remove it, then retry.");
                }
            }
            catch
            {
                handle.Dispose();
                throw;
            }

            return handle;
        }

        private static bool IsDirectory(SafeFileHandle handle)
        {
            if (!GetFileInformationByHandle(handle, out var information))
            {
                throw new Win32Exception(Marshal.GetLastWin32Error(), "failed to get file information");
            }

            return (information.FileAttributes & FileAttributeDirectory) != 0;
        }

        /// <summary>
        /// Return the owner of the object the handle refers to.
        /// </summary>
        private static SecurityIdentifier GetOwner(SafeFileHandle handle)
        {
            var error = GetSecurityInfo(handle, SeFileObject, OwnerSecurityInformation,
                out var owner, out _, out _, out _, out var securityDescriptor);
            if (error != 0)
            {
                throw new Win32Exception((int)error, "failed to read the owner");
            }

            try
            {
                // The owner points into the security descriptor, so it is read before freeing it.
                return new SecurityIdentifier(owner);
            }
            finally
            {
                LocalFree(securityDescriptor);
            }
        }

        /// <summary>
        /// Apply @security to the object the handle refers to, for a directory that already existed and
        /// to set SE_DACL_AUTO_INHERITED, which creating the directory does not.
        /// </summary>
        /// <remarks>
        /// The Go installer does the same, see createDirectoryWithSDDL in
        /// pkg/fleet/installer/paths/installer_paths_windows.go.
        /// </remarks>
        private static void SetSecurity(SafeFileHandle handle, DirectorySecurity security)
        {
            // The self relative form the Win32 functions expect. It is pinned because the pointers
            // read out of it below point into it.
            var binaryForm = security.GetSecurityDescriptorBinaryForm();

            var pinned = default(GCHandle);
            try
            {
                pinned = GCHandle.Alloc(binaryForm, GCHandleType.Pinned);
                var securityDescriptor = pinned.AddrOfPinnedObject();

                if (!GetSecurityDescriptorOwner(securityDescriptor, out var owner, out _) ||
                    !GetSecurityDescriptorGroup(securityDescriptor, out var group, out _) ||
                    !GetSecurityDescriptorDacl(securityDescriptor, out var daclPresent, out var dacl, out _) ||
                    !daclPresent)
                {
                    throw new Win32Exception(Marshal.GetLastWin32Error(),
                        "failed to read the security descriptor to apply");
                }

                var error = SetSecurityInfo(handle, SeFileObject,
                    OwnerSecurityInformation | GroupSecurityInformation |
                    DaclSecurityInformation | ProtectedDaclSecurityInformation,
                    owner, group, dacl, IntPtr.Zero);
                if (error != 0)
                {
                    throw new Win32Exception((int)error, "failed to apply the security descriptor");
                }
            }
            finally
            {
                if (pinned.IsAllocated)
                {
                    pinned.Free();
                }
            }
        }

        #endregion
    }

    /// <summary>
    /// A directory cannot be used. The message is shown to the user, so it explains how to resolve
    /// the problem.
    /// </summary>
    internal class SecureDirectoryException : Exception
    {
        public SecureDirectoryException(string message) : base(message)
        {
        }
    }
}
