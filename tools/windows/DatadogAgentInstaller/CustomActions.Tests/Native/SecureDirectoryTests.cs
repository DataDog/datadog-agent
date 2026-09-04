using System;
using System.IO;
using System.Linq;
using System.Security.AccessControl;
using System.Security.Principal;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using FluentAssertions;
using Moq;
using Xunit;

namespace CustomActions.Tests.Native
{
    public class SecureDirectoryTests : IDisposable
    {
        private readonly ISession _session = new Mock<ISession>().Object;
        private readonly string _root;

        public SecureDirectoryTests()
        {
            // Enabling this fails without the privilege, which would break the unelevated tests
            if (Elevation.IsAvailable)
            {
                TestDirectory.EnableSetOwnerPrivilege();
            }

            _root = Path.Combine(Path.GetTempPath(), $"SecureDirectoryTests-{Guid.NewGuid()}");
            Directory.CreateDirectory(_root);
        }

        public void Dispose()
        {
            try
            {
                // Take back the directories the tests made inaccessible, so they can be deleted
                foreach (var directory in Directory.GetDirectories(_root))
                {
                    TestDirectory.ReclaimForCleanup(directory);
                }

                Directory.Delete(_root, true);
            }
            catch (Exception)
            {
                // best effort cleanup
            }
        }

        private static SecurityIdentifier[] AllowedIdentities(string path)
        {
            return Directory.GetAccessControl(path, AccessControlSections.Access)
                .GetAccessRules(true, true, typeof(SecurityIdentifier))
                .Cast<FileSystemAccessRule>()
                .Select(r => (SecurityIdentifier)r.IdentityReference)
                .ToArray();
        }

        private static bool IsProtected(string path)
        {
            return Directory.GetAccessControl(path, AccessControlSections.Access).AreAccessRulesProtected;
        }

        private static string Dacl(string path)
        {
            return Directory.GetAccessControl(path, AccessControlSections.Access)
                .GetSecurityDescriptorSddlForm(AccessControlSections.Access);
        }

        [Fact]
        public void AdminOnlySecurity_Grants_Access_To_System_And_Administrators_Only()
        {
            var security = SecureDirectory.AdminOnlySecurity();

            security.AreAccessRulesProtected.Should().BeTrue();
            // The same owner and group the ConfigureUser custom action applies
            security.GetOwner(typeof(SecurityIdentifier)).Should().Be(TestDirectory.LocalSystem);
            security.GetGroup(typeof(SecurityIdentifier)).Should().Be(TestDirectory.LocalSystem);

            var rules = security.GetAccessRules(true, true, typeof(SecurityIdentifier))
                .Cast<FileSystemAccessRule>()
                .ToArray();
            rules.Select(r => r.IdentityReference)
                .Should().BeEquivalentTo(new[] { TestDirectory.LocalSystem, TestDirectory.Administrators });
            rules.Should().OnlyContain(r =>
                r.AccessControlType == AccessControlType.Allow &&
                r.FileSystemRights == FileSystemRights.FullControl &&
                r.InheritanceFlags == (InheritanceFlags.ObjectInherit | InheritanceFlags.ContainerInherit));
        }

        [Theory]
        // The installer resolves directory properties with a trailing separator, which would escape
        // the closing quote and leave the user with a command that does not run
        [InlineData(@"C:\ProgramData\Datadog\", @"""C:\ProgramData\Datadog""")]
        [InlineData(@"C:\ProgramData\Datadog", @"""C:\ProgramData\Datadog""")]
        [InlineData(@"D:\dd config\Datadog\", @"""D:\dd config\Datadog""")]
        public void PathForCommand_Quotes_The_Path_Without_Its_Trailing_Separator(string path, string expected)
        {
            SecureDirectory.PathForCommand(path).Should().Be(expected);
        }

        [Fact]
        public void IsTrustedOwner_Accepts_Only_System_Administrators_And_ContainerAdministrator()
        {
            SecureDirectory.IsTrustedOwner(TestDirectory.LocalSystem).Should().BeTrue();
            SecureDirectory.IsTrustedOwner(TestDirectory.Administrators).Should().BeTrue();
            SecureDirectory.IsTrustedOwner(TestDirectory.ContainerAdministrator).Should().BeTrue();
            SecureDirectory.IsTrustedOwner(TestDirectory.Everyone).Should().BeFalse();
            SecureDirectory.IsTrustedOwner(TestDirectory.UntrustedOwner).Should().BeFalse();
            // A fixed SID rather than the current identity: this suite can run as
            // ContainerAdministrator (see TestDirectory), which is now trusted.
            SecureDirectory.IsTrustedOwner(new SecurityIdentifier(WellKnownSidType.NetworkServiceSid, null))
                .Should().BeFalse();
        }

        [ElevatedFact]
        public void CreateAndSecure_Creates_The_Directory_With_The_AdminOnly_Dacl()
        {
            var path = Path.Combine(_root, "new");

            // The owner check may reject this under the CI identity, see TestDirectory. What must hold
            // either way is that the directory was created with the restricted DACL.
            try
            {
                SecureDirectory.CreateAndSecure(_session, path);
            }
            catch (SecureDirectoryException)
            {
                // covered by the tests below
            }

            Directory.Exists(path).Should().BeTrue();
            IsProtected(path).Should().BeTrue();
            AllowedIdentities(path).Should().BeEquivalentTo(
                new[] { TestDirectory.LocalSystem, TestDirectory.Administrators });
            // The owner is only set once the owner check has passed
            if (SecureDirectory.IsTrustedOwner(TestDirectory.OwnerOf(path)))
            {
                TestDirectory.OwnerOf(path).Should().Be(TestDirectory.LocalSystem);
            }
        }

        [ElevatedFact]
        public void CreateAndSecure_Resets_The_Permissions_Of_A_Directory_Owned_By_Administrators()
        {
            // A directory owned by Administrators is trusted even when it grants access to others,
            // Explorer offers to add that access when a split token administrator navigates to it.
            var path = TestDirectory.CreateOwnedBy(Path.Combine(_root, "trusted"),
                TestDirectory.Administrators, grantEveryone: true);
            AllowedIdentities(path).Should().Contain(TestDirectory.Everyone);

            SecureDirectory.CreateAndSecure(_session, path);

            IsProtected(path).Should().BeTrue();
            AllowedIdentities(path).Should().BeEquivalentTo(
                new[] { TestDirectory.LocalSystem, TestDirectory.Administrators });
            // P: does not inherit from the parent, AI: the ACEs propagate to children
            Dacl(path).Should().StartWith("D:PAI");
            // The same owner the ConfigureUser custom action applies to the configuration directory
            TestDirectory.OwnerOf(path).Should().Be(TestDirectory.LocalSystem);
        }

        [ElevatedFact]
        public void CreateAndSecure_Rejects_A_Directory_Owned_By_Another_User()
        {
            var path = TestDirectory.CreateOwnedBy(Path.Combine(_root, "pre-created"),
                TestDirectory.UntrustedOwner, grantEveryone: true);

            var act = () => SecureDirectory.CreateAndSecure(_session, path);

            act.Should().Throw<SecureDirectoryException>()
                .WithMessage("*has unexpected owner*")
                .And.Message.Should().Contain("takeown.exe");
            // The permissions of a rejected directory are left alone, the install stops instead.
            AllowedIdentities(path).Should().Contain(TestDirectory.Everyone);
            IsProtected(path).Should().BeFalse();
        }

        [ElevatedFact]
        public void CreateAndSecure_Does_Not_Follow_A_Reparse_Point()
        {
            var target = TestDirectory.CreateOwnedBy(Path.Combine(_root, "junction-target"),
                TestDirectory.UntrustedOwner, grantEveryone: true);
            var targetDacl = Dacl(target);
            var path = Path.Combine(_root, "junction");
            TestDirectory.CreateJunction(path, target);
            TestDirectory.SetOwner(path, TestDirectory.Administrators);

            SecureDirectory.CreateAndSecure(_session, path);

            Dacl(target).Should().Be(targetDacl, "the permissions of the junction target must not be modified");
            TestDirectory.OwnerOf(target).Should().Be(TestDirectory.UntrustedOwner,
                "the owner of the junction target must not be modified");
        }

        [ElevatedFact]
        public void CreateAndSecure_Rejects_A_Reparse_Point_Owned_By_Another_User()
        {
            var target = TestDirectory.CreateOwnedBy(Path.Combine(_root, "junction-target"),
                TestDirectory.Administrators);
            var path = Path.Combine(_root, "junction");
            TestDirectory.CreateJunction(path, target);
            TestDirectory.SetOwner(path, TestDirectory.UntrustedOwner);

            var act = () => SecureDirectory.CreateAndSecure(_session, path);

            act.Should().Throw<SecureDirectoryException>().WithMessage("*has unexpected owner*");
        }

        [ElevatedFact]
        public void CreateIfMissing_Creates_The_Directory_With_The_AdminOnly_Dacl()
        {
            var path = Path.Combine(_root, "new");

            // The owner check may reject this under the CI identity, see TestDirectory. What must hold
            // either way is that the directory was created with the restricted DACL.
            try
            {
                SecureDirectory.CreateIfMissing(_session, path);
            }
            catch (SecureDirectoryException)
            {
                // covered by CreateAndSecure's own tests, the creation path is shared
            }

            Directory.Exists(path).Should().BeTrue();
            IsProtected(path).Should().BeTrue();
            AllowedIdentities(path).Should().BeEquivalentTo(
                new[] { TestDirectory.LocalSystem, TestDirectory.Administrators });
        }

        [ElevatedFact]
        public void CreateIfMissing_Does_Not_Reset_The_Permissions_Of_An_Existing_Directory()
        {
            // Unlike CreateAndSecure, an already-trusted directory's permissions (e.g. a grant added
            // for ddagentuser after it was first created) must survive untouched.
            var path = TestDirectory.CreateOwnedBy(Path.Combine(_root, "trusted"),
                TestDirectory.Administrators, grantEveryone: true);
            var before = Dacl(path);

            SecureDirectory.CreateIfMissing(_session, path);

            Dacl(path).Should().Be(before);
        }

        [ElevatedFact]
        public void CreateIfMissing_Rejects_A_Directory_Owned_By_Another_User()
        {
            var path = TestDirectory.CreateOwnedBy(Path.Combine(_root, "pre-created"),
                TestDirectory.UntrustedOwner, grantEveryone: true);

            var act = () => SecureDirectory.CreateIfMissing(_session, path);

            act.Should().Throw<SecureDirectoryException>()
                .WithMessage("*has unexpected owner*")
                .And.Message.Should().Contain("takeown.exe");
        }

        [Theory]
        // A missing directory, and a missing parent, report different errors from Windows
        [InlineData("missing")]
        [InlineData(@"missing\deeper")]
        public void AssertSecureOwner_Accepts_A_Missing_Directory(string relativePath)
        {
            var act = () => SecureDirectory.AssertSecureOwner(_session, Path.Combine(_root, relativePath));

            act.Should().NotThrow();
        }

        [Fact]
        public void AssertSecureOwner_Rejects_A_File_At_The_Path()
        {
            // Directory.Exists would report this as missing, and the install would fail later
            var path = Path.Combine(_root, "a-file");
            File.WriteAllText(path, "not a directory");

            var act = () => SecureDirectory.AssertSecureOwner(_session, path);

            act.Should().Throw<SecureDirectoryException>().WithMessage("*is not a directory*");
        }

        [Fact]
        public void AssertSecureOwner_Rejects_A_Path_It_Cannot_Open()
        {
            // Built by hand because Path.Combine rejects the name
            var path = _root + @"\in|valid";

            var act = () => SecureDirectory.AssertSecureOwner(_session, path);

            act.Should().Throw<SecureDirectoryException>().WithMessage("*Failed to open*");
        }

        [ElevatedFact]
        public void AssertSecureOwner_Accepts_A_Directory_Owned_By_Administrators()
        {
            var path = TestDirectory.CreateOwnedBy(Path.Combine(_root, "trusted"), TestDirectory.Administrators);

            var act = () => SecureDirectory.AssertSecureOwner(_session, path);

            act.Should().NotThrow();
        }

        [ElevatedFact]
        public void AssertSecureOwner_Rejects_A_Directory_Owned_By_Another_User()
        {
            var path = TestDirectory.CreateOwnedBy(Path.Combine(_root, "existing"),
                TestDirectory.UntrustedOwner, grantEveryone: true);
            var before = Dacl(path);

            var act = () => SecureDirectory.AssertSecureOwner(_session, path);

            act.Should().Throw<SecureDirectoryException>().WithMessage("*has unexpected owner*");
            // The check must not change anything, it runs before the install makes any change.
            Dacl(path).Should().Be(before);
        }
    }
}
