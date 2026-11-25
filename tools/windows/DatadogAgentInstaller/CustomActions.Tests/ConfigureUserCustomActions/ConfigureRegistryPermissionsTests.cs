using System.Collections.Generic;
using System.Linq;
using System.Security.AccessControl;
using System.Security.Principal;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using FluentAssertions;
using Moq;

namespace CustomActions.Tests.ConfigureUserCustomActions
{
    public class ConfigureRegistryPermissionsTests
    {
        public ConfigureUserCustomActionsTestSetup Test { get; } = new();

        [ElevatedFact]
        public void ConfigureRegistryPermissions_Sets_LocalSystem_Owner_Before_Admin_Only_Rules()
        {
            var order = new List<string>();
            RegistrySecurity rules = null;
            var key = new Mock<IRegistryKey>();
            key.Setup(k => k.SetAccessControl(It.IsAny<RegistrySecurity>()))
                .Callback<RegistrySecurity>(s =>
                {
                    rules = s;
                    order.Add("rules");
                });
            Test.RegistryServices
                .Setup(r => r.CreateRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey))
                .Returns(key.Object);

            RegistrySecurity owner = null;
            var ownerKey = new Mock<IRegistryKey>();
            ownerKey.Setup(k => k.SetAccessControl(It.IsAny<RegistrySecurity>()))
                .Callback<RegistrySecurity>(s =>
                {
                    owner = s;
                    order.Add("owner");
                });
            Test.RegistryServices
                .Setup(r => r.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey,
                    It.Is<RegistryRights>(rights => rights.HasFlag(RegistryRights.TakeOwnership))))
                .Returns(ownerKey.Object);

            Test.Create().ConfigureRegistryPermissions();

            rules.Should().NotBeNull();
            rules.GetAccessRules(true, false, typeof(SecurityIdentifier))
                .Cast<RegistryAccessRule>()
                .Select(r => r.IdentityReference)
                .Should().BeEquivalentTo(new[]
                {
                    new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null),
                    new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid, null),
                });

            owner.Should().NotBeNull();
            owner.GetOwner(typeof(SecurityIdentifier)).Should()
                .Be(new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null));
            owner.GetAccessRules(true, true, typeof(SecurityIdentifier)).Count.Should().Be(0);
            ownerKey.Verify(k => k.Dispose());
            order.Should().Equal("owner", "rules");
        }
    }
}
