using System.DirectoryServices.ActiveDirectory;
using System.Security.Principal;
using AutoFixture;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Moq;

namespace CustomActions.Tests.ProcessUserCustomActions
{
    public class ProcessUserCustomActionsTestSetup : SessionTestBaseSetup
    {
        private readonly Fixture _fixture = new();

        public Mock<INativeMethods> NativeMethods { get; } = new();
        public Mock<IServiceController> ServiceController { get; } = new();
        public Mock<IRegistryServices> RegistryServices { get; } = new();

        public ProcessUserCustomActionsTestSetup()
        {
            WithLocalSystem();

            // By default computers are not domain-joined
            NativeMethods.Setup(n => n.IsDomainController()).Returns(false);
            NativeMethods.Setup(n => n.IsReadOnlyDomainController()).Returns(false);
            NativeMethods.Setup(n => n.GetComputerDomain()).Throws<ActiveDirectoryObjectNotFoundException>();
            ServiceController.SetupGet(s => s.Services).Returns(new WindowsService[] { });
        }

        public Datadog.CustomActions.ProcessUserCustomActions Create()
        {
            return new Datadog.CustomActions.ProcessUserCustomActions(
                Session.Object,
                NativeMethods.Object,
                ServiceController.Object,
                RegistryServices.Object
            );
        }

        public ProcessUserCustomActionsTestSetup WithLocalSystem()
        {
            var userSid = new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null);
            NativeMethods.Setup(n => n.LookupAccountName(
                    "NT AUTHORITY\\SYSTEM",
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(new LookupAccountNameDelegate(
                    (
                        string _,
                        out string user,
                        out string domain,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse
                    ) =>
                    {
                        user = "SYSTEM";
                        domain = "NT AUTHORITY";
                        sid = userSid;
                        nameUse = SID_NAME_USE.SidTypeWellKnownGroup;
                    }))
                .Returns(true);
            NativeMethods.Setup(n => n.IsServiceAccount(userSid)).Returns(true);

            return this;
        }

        public ProcessUserCustomActionsTestSetup WithDatadogAgentService()
        {
            var service = new Mock<IWindowsService>();
            service.SetupGet(s => s.DisplayName).Returns("Datadog Agent");
            service.SetupGet(s => s.ServiceName).Returns("datadogagent");
            ServiceController.SetupGet(s => s.Services).Returns(new[]
            {
                service.Object
            });

            return this;
        }

        public ProcessUserCustomActionsTestSetup WithDomainController(string domain = null)
        {
            domain ??= _fixture.Create<string>();

            NativeMethods.Setup(n => n.IsDomainController()).Returns(true);
            NativeMethods.Setup(n => n.IsReadOnlyDomainController()).Returns(false);
            NativeMethods.Setup(n => n.GetComputerDomain()).Returns(domain);

            return this;
        }

        public ProcessUserCustomActionsTestSetup WithReadOnlyDomainController(string domain = null)
        {
            domain ??= _fixture.Create<string>();

            NativeMethods.Setup(n => n.IsDomainController()).Returns(true);
            NativeMethods.Setup(n => n.IsReadOnlyDomainController()).Returns(true);
            NativeMethods.Setup(n => n.GetComputerDomain()).Returns(domain);

            return this;
        }

        public ProcessUserCustomActionsTestSetup WithDomainClient(string domain = null)
        {
            domain ??= _fixture.Create<string>();

            NativeMethods.Setup(n => n.IsDomainController()).Returns(false);
            NativeMethods.Setup(n => n.GetComputerDomain()).Returns(domain);

            return this;
        }

        public ProcessUserCustomActionsTestSetup WithLocalUser(
            string userDomain,
            string userName,
            SID_NAME_USE userType = SID_NAME_USE.SidTypeUser,
            SecurityIdentifier userSid = null)
        {
            if (userSid == null)
            {
                userSid = new SecurityIdentifier($"S-1-0-{_fixture.Create<uint>()}");
            }

            NativeMethods.Setup(n => n.IsServiceAccount(userSid)).Returns(false);
            NativeMethods.Setup(n => n.IsDomainAccount(userSid)).Returns(false);
            NativeMethods.Setup(n => n.LookupAccountName(
                    $"{userDomain}\\{userName}",
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(new LookupAccountNameDelegate(
                    (
                        string _,
                        out string user,
                        out string domain,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse
                    ) =>
                    {
                        user = userName;
                        domain = userDomain;
                        sid = userSid;
                        nameUse = userType;
                    }))
                .Returns(true);

            return this;
        }

        public ProcessUserCustomActionsTestSetup WithDomainUser(
            string userName = null,
            SID_NAME_USE userType = SID_NAME_USE.SidTypeUser)
        {
            userName ??= _fixture.Create<string>();
            var userDomain = NativeMethods.Object.GetComputerDomain();
            var userSid = new SecurityIdentifier($"S-1-0-{_fixture.Create<uint>()}");

            NativeMethods.Setup(n => n.IsServiceAccount(userSid)).Returns(false);
            NativeMethods.Setup(n => n.IsDomainAccount(userSid)).Returns(true);
            NativeMethods.Setup(n => n.LookupAccountName(
                    $"{userDomain}\\{userName}",
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(new LookupAccountNameDelegate(
                    (
                        string _,
                        out string user,
                        out string domain,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse
                    ) =>
                    {
                        user = userName;
                        domain = userDomain;
                        sid = userSid;
                        nameUse = userType;
                    }))
                .Returns(true);

            return this;
        }

        public ProcessUserCustomActionsTestSetup WithManagedServiceAccount(
            string userName,
            SID_NAME_USE userType = SID_NAME_USE.SidTypeUser)
        {
            var userDomain = NativeMethods.Object.GetComputerDomain();
            var userSid = new SecurityIdentifier($"S-1-0-{_fixture.Create<uint>()}");

            NativeMethods.Setup(n => n.IsServiceAccount(userSid)).Returns(true);
            NativeMethods.Setup(n => n.IsDomainAccount(userSid)).Returns(true);
            NativeMethods.Setup(n => n.LookupAccountName(
                    $"{userDomain}\\{userName}",
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(new LookupAccountNameDelegate(
                    (
                        string _,
                        out string user,
                        out string domain,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse
                    ) =>
                    {
                        user = userName;
                        domain = userDomain;
                        sid = userSid;
                        nameUse = userType;
                    }))
                .Returns(true);

            return this;
        }

        public ProcessUserCustomActionsTestSetup WithCurrentUser(
            string userName,
            SecurityIdentifier userSID = null)
        {
            if (userSID == null)
            {
                userSID = new SecurityIdentifier($"S-1-0-{_fixture.Create<uint>()}");
            }

            NativeMethods.Setup(n => n.GetCurrentUser(
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny))
                .Callback(new GetCurrentUserDelegate(
                    (
                        out string user,
                        out SecurityIdentifier sid
                    ) =>
                    {
                        user = userName;
                        sid = userSID;
                    }));

            return this;
        }

        public ProcessUserCustomActionsTestSetup WithPreviousAgentUser(
            string userDomain,
            string userName)
        {
            var mockRegKey = _fixture.Create<Mock<IRegistryKey>>();
            RegistryServices.Setup(
                r => r.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey)).Returns(mockRegKey.Object);
            mockRegKey.Setup(r => r.GetValue("installedDomain")).Returns(userDomain);
            mockRegKey.Setup(r => r.GetValue("installedUser")).Returns(userName);

            return this;
        }
    }
}
