using System;
using System.IO;
using System.Linq;
using System.Security.AccessControl;
using System.Security.Cryptography;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Newtonsoft.Json;
using WixToolset.Dtf.WindowsInstaller;
using FileAttributes = System.IO.FileAttributes;

namespace Datadog.CustomActions
{
    public static class ParConfigMigrationCustomActions
    {
        internal const string RelativePath = @"private-action-runner\powershell-script-config.yaml";
        private const AccessControlSections Sections = AccessControlSections.Owner | AccessControlSections.Group | AccessControlSections.Access;

        private sealed class Metadata
        {
            public string Path { get; set; }
            public string Security { get; set; }
            public FileAttributes Attributes { get; set; }
        }

        internal sealed class Snapshot
        {
            private readonly ISession _session;
            private readonly string _live;
            private readonly string _store;
            private string Data => System.IO.Path.Combine(_store, "config");
            private string Manifest => System.IO.Path.Combine(_store, "metadata.json");

            internal Snapshot(ISession session, string configRoot, string store)
            {
                _session = session;
                _live = System.IO.Path.Combine(System.IO.Path.GetFullPath(configRoot), RelativePath);
                _store = store;
            }

            internal bool Pending => File.Exists(Data) || File.Exists(Manifest);
            internal bool Complete => File.Exists(Manifest);

            private void ValidatePaths()
            {
                foreach (var path in new[] { System.IO.Path.GetDirectoryName(System.IO.Path.GetDirectoryName(_live)), System.IO.Path.GetDirectoryName(_live), _store })
                {
                    SecureDirectory.AssertSecureOwner(_session, path);
                    try
                    {
                        if ((File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0)
                        {
                            throw new IOException("PAR config migration does not support reparse directories");
                        }
                    }
                    catch (FileNotFoundException) { }
                    catch (DirectoryNotFoundException) { }
                }
            }

            private Metadata ReadMetadata()
            {
                ValidatePaths();
                foreach (var path in new[] { Data, Manifest })
                {
                    if ((File.GetAttributes(path) & (FileAttributes.ReparsePoint | FileAttributes.Directory)) != 0)
                    {
                        throw new IOException("Invalid PAR config migration snapshot");
                    }
                }
                var metadata = JsonConvert.DeserializeObject<Metadata>(File.ReadAllText(Manifest));
                if (metadata == null || !string.Equals(metadata.Path, _live, StringComparison.OrdinalIgnoreCase))
                {
                    throw new IOException("PAR config migration destination does not match the retained snapshot");
                }
                return metadata;
            }

            internal void Capture(bool rollbackDisabled = false)
            {
                ValidatePaths();
                if (Pending)
                {
                    RequireRollback(rollbackDisabled);
                    var retained = ReadMetadata();
                    FileAttributes liveAttributes;
                    try { liveAttributes = File.GetAttributes(_live); }
                    catch (FileNotFoundException) { return; }
                    catch (DirectoryNotFoundException) { return; }
                    if ((liveAttributes & (FileAttributes.ReparsePoint | FileAttributes.Directory)) != 0 ||
                        liveAttributes != retained.Attributes || !SameContent(_live, Data) ||
                        !SameSecurity(retained.Security, File.GetAccessControl(_live, Sections).GetSecurityDescriptorSddlForm(Sections)))
                    {
                        throw new IOException("PAR config changed while migration is pending; manual recovery is required");
                    }
                    return;
                }
                FileAttributes attributes;
                try { attributes = File.GetAttributes(_live); }
                catch (FileNotFoundException) { return; }
                catch (DirectoryNotFoundException) { return; }
                if ((attributes & (FileAttributes.ReparsePoint | FileAttributes.Directory)) != 0)
                {
                    throw new IOException("PAR config migration requires a regular file");
                }
                if (!SupportedAttributes(attributes))
                {
                    throw new IOException("PAR config migration does not support these file attributes");
                }
                RequireRollback(rollbackDisabled);
                var metadata = new Metadata
                {
                    Path = _live,
                    Security = File.GetAccessControl(_live, Sections).GetSecurityDescriptorSddlForm(Sections),
                    Attributes = attributes
                };
                SecureDirectory.CreateAndSecure(_session, _store);
                // Snapshot bytes never enter MSI properties or installer logs.
                using (var input = new FileStream(_live, FileMode.Open, FileAccess.Read, FileShare.Read))
                using (var output = new FileStream(Data, FileMode.CreateNew, FileAccess.Write, FileShare.None))
                {
                    input.CopyTo(output);
                    output.Flush(true);
                }
                File.WriteAllText(Manifest + ".tmp", JsonConvert.SerializeObject(metadata));
                File.Move(Manifest + ".tmp", Manifest);
            }

            private static void RequireRollback(bool disabled)
            {
                if (disabled) { throw new IOException("PAR config migration requires Windows Installer rollback to be enabled"); }
            }

            internal void Restore()
            {
                if (!Pending) { return; }
                var metadata = ReadMetadata();
                new Win32NativeMethods().EnablePrivilege("SeRestorePrivilege");
                var security = new FileSecurity();
                security.SetSecurityDescriptorSddlForm(metadata.Security, Sections);
                var temporary = _live + ".migration-" + Guid.NewGuid().ToString("N");
                try
                {
                    // Secure the temporary file before writing customer bytes.
                    using (var input = new FileStream(Data, FileMode.Open, FileAccess.Read, FileShare.Read))
                    using (var output = new FileStream(temporary, FileMode.CreateNew, FileSystemRights.FullControl,
                        FileShare.None, 4096, FileOptions.None, security))
                    {
                        input.CopyTo(output);
                        output.Flush(true);
                    }
                    if (File.Exists(_live))
                    {
                        if ((File.GetAttributes(_live) & (FileAttributes.ReparsePoint | FileAttributes.Directory)) != 0)
                        {
                            throw new IOException("Unsafe PAR config restoration destination");
                        }
                        File.SetAttributes(_live, FileAttributes.Normal);
                        File.Replace(temporary, _live, null);
                    }
                    else
                    {
                        File.Move(temporary, _live);
                    }
                    // ReplaceFile can retain the replaced file's security descriptor.
                    File.SetAccessControl(_live, security);
                    File.SetAttributes(_live, metadata.Attributes);
                    if (File.GetAttributes(_live) != metadata.Attributes)
                    {
                        throw new IOException("PAR config attribute restoration verification failed");
                    }
                    if (!SameContent(_live, Data))
                    {
                        throw new IOException("PAR config byte restoration verification failed");
                    }
                    var actual = File.GetAccessControl(_live, Sections).GetSecurityDescriptorSddlForm(Sections);
                    if (!SameSecurity(metadata.Security, actual))
                    {
                        throw new IOException("PAR config security restoration verification failed");
                    }
                }
                finally
                {
                    if (File.Exists(temporary)) { File.Delete(temporary); }
                }
            }

            internal void Cleanup()
            {
                if (!Directory.Exists(_store)) { return; }
                ValidatePaths();
                File.Delete(Data);
                File.Delete(Manifest);
                Directory.Delete(_store);
            }

            private static bool SameContent(string a, string b)
            {
                using (var hash = SHA256.Create())
                using (var first = File.OpenRead(a))
                using (var second = File.OpenRead(b))
                {
                    return Convert.ToBase64String(hash.ComputeHash(first)) == Convert.ToBase64String(hash.ComputeHash(second));
                }
            }
        }

        internal static bool SupportedAttributes(FileAttributes attributes)
        {
            const FileAttributes supported = FileAttributes.Archive | FileAttributes.Hidden | FileAttributes.System |
                FileAttributes.ReadOnly | FileAttributes.Normal | FileAttributes.Temporary | FileAttributes.NotContentIndexed;
            return (attributes & ~supported) == 0;
        }

        internal static bool SameSecurity(string expected, string actual)
        {
            var before = new RawSecurityDescriptor(expected);
            var after = new RawSecurityDescriptor(actual);
            // On a protected file, AI records historical processing, not inherited permissions.
            var ignored = (before.ControlFlags & ControlFlags.DiscretionaryAclProtected) != 0
                ? ControlFlags.DiscretionaryAclAutoInherited : 0;
            if (!before.Owner.Equals(after.Owner) || !before.Group.Equals(after.Group) ||
                (before.ControlFlags & ~ignored) != (after.ControlFlags & ~ignored)) { return false; }
            if (before.DiscretionaryAcl == null || after.DiscretionaryAcl == null)
            {
                return before.DiscretionaryAcl == null && after.DiscretionaryAcl == null;
            }
            var first = new byte[before.DiscretionaryAcl.BinaryLength];
            var second = new byte[after.DiscretionaryAcl.BinaryLength];
            before.DiscretionaryAcl.GetBinaryForm(first, 0);
            after.DiscretionaryAcl.GetBinaryForm(second, 0);
            return first.SequenceEqual(second);
        }

        private static Snapshot ForSession(ISession session)
        {
            var code = Guid.Parse(session.Property("ProductCode")).ToString("N");
            var parent = System.IO.Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "Datadog-PAR-config-migration");
            SecureDirectory.CreateAndSecure(session, parent);
            return new Snapshot(session, session.Property("APPLICATIONDATADIRECTORY"), System.IO.Path.Combine(parent, code));
        }

        internal static bool IsPending(ISession session)
        {
            if (string.IsNullOrEmpty(session.Property("WIX_UPGRADE_DETECTED"))) { return false; }
            return ForSession(session).Pending;
        }

        private static ActionResult Run(Session session, Action<Snapshot> action)
        {
            var wrapped = new SessionWrapper(session);
            try
            {
                action(ForSession(wrapped));
                return ActionResult.Success;
            }
            catch (Exception e)
            {
                wrapped.Log($"PAR config migration failed: {e.Message}");
                return ActionResult.Failure;
            }
        }

        public static ActionResult Capture(Session session) => Run(session,
            snapshot => snapshot.Capture(new SessionWrapper(session).Property("RollbackDisabled") == "1"));
        public static ActionResult Restore(Session session) => Run(session, snapshot => snapshot.Restore());
        public static ActionResult Rollback(Session session)
        {
            return Run(session, snapshot =>
            {
                // Capture may have failed before publishing a complete snapshot.
                if (!snapshot.Complete) { return; }
                snapshot.Restore();
            });
        }
        public static ActionResult Cleanup(Session session) => Run(session, snapshot => snapshot.Cleanup());
    }
}
