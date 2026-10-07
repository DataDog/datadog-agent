using System;
using System.IO;
using System.Security.AccessControl;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Moq;
using Xunit;

namespace CustomActions.Tests
{
    public class ParConfigMigrationTests
    {
        [ElevatedTheory]
        [InlineData("customer config\n", false, true)]
        [InlineData("", false, true)]
        [InlineData("customer config\n", true, true)]
        [InlineData("customer config\n", false, false)]
        public void SnapshotRestoresBytesAndProtectedSecurity(string content, bool readOnly, bool protectedDacl)
        {
            var root = Path.Combine(Path.GetTempPath(), "ParMigration-" + Guid.NewGuid());
            var config = Path.Combine(root, "config");
            var live = Path.Combine(config, ParConfigMigrationCustomActions.RelativePath);
            TestDirectory.CreateOwnedBy(config, TestDirectory.Administrators);
            TestDirectory.CreateOwnedBy(Path.GetDirectoryName(live), TestDirectory.Administrators);
            var store = Path.Combine(root, "snapshot");
            var session = new Mock<ISession>();
            try
            {
                File.WriteAllText(live, content);
                var security = File.GetAccessControl(live);
                security.SetAccessRuleProtection(protectedDacl, true);
                security.SetOwner(TestDirectory.LocalSystem);
                File.SetAccessControl(live, security);
                var original = File.GetAccessControl(live, AccessControlSections.Owner | AccessControlSections.Group | AccessControlSections.Access).GetSecurityDescriptorSddlForm(
                    AccessControlSections.Owner | AccessControlSections.Group | AccessControlSections.Access);
                if (readOnly) { File.SetAttributes(live, FileAttributes.ReadOnly); }
                var snapshot = new ParConfigMigrationCustomActions.Snapshot(session.Object, config, store);
                snapshot.Capture();
                Assert.True(snapshot.Pending);
                File.SetAttributes(live, FileAttributes.Normal);
                File.Delete(live);
                snapshot.Restore();
                Assert.Equal(content, File.ReadAllText(live));
                Assert.True(ParConfigMigrationCustomActions.SameSecurity(original,
                    File.GetAccessControl(live, AccessControlSections.Owner | AccessControlSections.Group | AccessControlSections.Access)
                        .GetSecurityDescriptorSddlForm(AccessControlSections.Owner | AccessControlSections.Group | AccessControlSections.Access)));
                Assert.Equal(protectedDacl, File.GetAccessControl(live).AreAccessRulesProtected);
                Assert.Equal(readOnly, (File.GetAttributes(live) & FileAttributes.ReadOnly) != 0);
                snapshot.Restore();
                snapshot.Cleanup();
                Assert.False(snapshot.Pending);
            }
            finally
            {
                if (File.Exists(live)) { File.SetAttributes(live, FileAttributes.Normal); }
                Directory.Delete(root, true);
            }
        }

        [Fact]
        public void UnsupportedStorageAttributesAreRejected()
        {
            Assert.True(ParConfigMigrationCustomActions.SupportedAttributes(FileAttributes.ReadOnly | FileAttributes.Archive));
            Assert.False(ParConfigMigrationCustomActions.SupportedAttributes(FileAttributes.Encrypted));
            Assert.False(ParConfigMigrationCustomActions.SupportedAttributes(FileAttributes.Compressed));
            Assert.False(ParConfigMigrationCustomActions.SupportedAttributes(FileAttributes.SparseFile));
            Assert.False(ParConfigMigrationCustomActions.SupportedAttributes(FileAttributes.ReparsePoint));
        }

        [Fact]
        public void SecurityComparisonDoesNotIgnorePermissionsOrProtection()
        {
            const string original = "O:SYG:SYD:PAI(A;;FA;;;SY)";
            Assert.True(ParConfigMigrationCustomActions.SameSecurity(original, "O:SYG:SYD:P(A;;FA;;;SY)"));
            Assert.False(ParConfigMigrationCustomActions.SameSecurity(original, "O:SYG:SYD:P(A;;FR;;;SY)"));
            Assert.False(ParConfigMigrationCustomActions.SameSecurity(original, "O:SYG:SYD:AI(A;;FA;;;SY)"));
            Assert.False(ParConfigMigrationCustomActions.SameSecurity("O:SYG:SYD:AI(A;;FA;;;SY)", "O:SYG:SYD:(A;;FA;;;SY)"));
        }

        [ElevatedFact]
        public void SnapshotRequiresRollbackOnlyWhenCustomerDataExists()
        {
            var root = Path.Combine(Path.GetTempPath(), "ParMigration-" + Guid.NewGuid());
            var config = Path.Combine(root, "config");
            var live = Path.Combine(config, ParConfigMigrationCustomActions.RelativePath);
            TestDirectory.CreateOwnedBy(config, TestDirectory.Administrators);
            TestDirectory.CreateOwnedBy(Path.GetDirectoryName(live), TestDirectory.Administrators);
            try
            {
                var snapshot = new ParConfigMigrationCustomActions.Snapshot(new Mock<ISession>().Object, config, Path.Combine(root, "snapshot"));
                snapshot.Capture(rollbackDisabled: true);
                Assert.False(snapshot.Pending);
                File.WriteAllText(live, "customer data");
                Assert.Throws<IOException>(() => snapshot.Capture(rollbackDisabled: true));
                Assert.Equal("customer data", File.ReadAllText(live));
                Assert.False(snapshot.Pending);
            }
            finally { Directory.Delete(root, true); }
        }

        [ElevatedFact]
        public void SnapshotRejectsChangedConfigOnRetry()
        {
            var root = Path.Combine(Path.GetTempPath(), "ParMigration-" + Guid.NewGuid());
            var config = Path.Combine(root, "config");
            var live = Path.Combine(config, ParConfigMigrationCustomActions.RelativePath);
            TestDirectory.CreateOwnedBy(config, TestDirectory.Administrators);
            TestDirectory.CreateOwnedBy(Path.GetDirectoryName(live), TestDirectory.Administrators);
            try
            {
                File.WriteAllText(live, "original");
                var snapshot = new ParConfigMigrationCustomActions.Snapshot(new Mock<ISession>().Object, config, Path.Combine(root, "snapshot"));
                snapshot.Capture();
                var attributes = File.GetAttributes(live);
                File.WriteAllText(live, "new customer edit");
                Assert.Throws<IOException>(() => snapshot.Capture());
                Assert.Equal("new customer edit", File.ReadAllText(live));
                Assert.True(snapshot.Pending);
                File.WriteAllText(live, "original");
                File.SetAttributes(live, FileAttributes.ReadOnly);
                Assert.Throws<IOException>(() => snapshot.Capture());
                File.SetAttributes(live, attributes);
                var security = File.GetAccessControl(live);
                security.SetAccessRuleProtection(true, true);
                File.SetAccessControl(live, security);
                Assert.Throws<IOException>(() => snapshot.Capture());
                File.Delete(live);
                Directory.CreateDirectory(live);
                Assert.Throws<IOException>(() => snapshot.Capture());
            }
            finally
            {
                if (File.Exists(live)) { File.SetAttributes(live, FileAttributes.Normal); }
                Directory.Delete(root, true);
            }
        }

        [ElevatedFact]
        public void SnapshotRejectsReparseParentBeforeCapture()
        {
            var root = Path.Combine(Path.GetTempPath(), "ParMigration-" + Guid.NewGuid());
            var config = Path.Combine(root, "config");
            var target = Path.Combine(root, "target");
            TestDirectory.CreateOwnedBy(config, TestDirectory.Administrators);
            TestDirectory.CreateOwnedBy(target, TestDirectory.Administrators);
            var link = Path.Combine(config, "private-action-runner");
            TestDirectory.CreateJunction(link, target);
            try
            {
                var snapshot = new ParConfigMigrationCustomActions.Snapshot(new Mock<ISession>().Object, config, Path.Combine(root, "snapshot"));
                Assert.Throws<IOException>(() => snapshot.Capture());
                Assert.False(snapshot.Pending);
            }
            finally
            {
                Directory.Delete(link);
                Directory.Delete(root, true);
            }
        }
    }
}
