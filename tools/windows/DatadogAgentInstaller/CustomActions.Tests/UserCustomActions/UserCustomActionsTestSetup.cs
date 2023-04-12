using System;
using System.Collections.Generic;
using System.DirectoryServices.ActiveDirectory;
using System.Security.Principal;
using AutoFixture;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Moq;

namespace CustomActions.Tests.UserCustomActions
{
    public class UserCustomActionsTestSetup
    {
        private readonly Fixture _fixture = new();
        private readonly Dictionary<string, string> _properties = new();

        public IReadOnlyDictionary<string, string> Properties => _properties;

        public Mock<ISession> Session { get; } = new();
        public Mock<INativeMethods> NativeMethods { get; } = new();
        public Mock<IRegistryServices> RegistryServices { get; } = new();
        public Mock<IDirectoryServices> DirectoryServices { get; } = new();
        public Mock<IFileServices> FileServices { get; } = new();
        public Mock<IServiceController> ServiceController { get; } = new();

        public UserCustomActionsTestSetup()
        {
            WithLocalSystem();
            
            // By default computers are not domain-joined
            NativeMethods.Setup(n => n.IsDomainController()).Returns(false);
            NativeMethods.Setup(n => n.GetComputerDomain()).Throws<ActiveDirectoryObjectNotFoundException>();

            Session
                .SetupSet(s => s[It.IsAny<string>()] = It.IsAny<string>())
                .Callback((string key, string value) => _properties[key] = value);
        }

        public Datadog.CustomActions.UserCustomActions Create()
        {
            return new Datadog.CustomActions.UserCustomActions(
                Session.Object,
                NativeMethods.Object,
                RegistryServices.Object,
                DirectoryServices.Object,
                FileServices.Object,
                ServiceController.Object
            );
        }

        public UserCustomActionsTestSetup WithLocalSystem()
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

        public UserCustomActionsTestSetup WithDatadogAgentService()
        {
            ServiceController.Setup(s => s.GetServiceNames()).Returns(new[] { Tuple.Create("datadogagent", "Datadog Agent") });
            ServiceController.Setup(s => s.ServiceExists("datadogagent")).Returns(true);

            return this;
        }

        public UserCustomActionsTestSetup WithDomainController(string domain = null)
        {
            domain ??= _fixture.Create<string>();

            NativeMethods.Setup(n => n.IsDomainController()).Returns(true);
            NativeMethods.Setup(n => n.GetComputerDomain()).Returns(domain);

            return this;
        }

        public UserCustomActionsTestSetup WithDomainClient(string domain = null)
        {
            domain ??= _fixture.Create<string>();

            NativeMethods.Setup(n => n.IsDomainController()).Returns(false);
            NativeMethods.Setup(n => n.GetComputerDomain()).Returns(domain);

            return this;
        }

        public UserCustomActionsTestSetup WithLocalUser(
            string userDomain,
            string userName,
            SID_NAME_USE userType = SID_NAME_USE.SidTypeUser)
        {
            var userSid = new SecurityIdentifier($"S-1-0-{_fixture.Create<uint>()}");

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

        public UserCustomActionsTestSetup WithDomainUser(
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

        public UserCustomActionsTestSetup WithManagedServiceAccount(
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
    }
}
