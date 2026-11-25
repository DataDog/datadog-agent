using AutoFixture;
using CustomActions.Tests.ProcessUserCustomActions;
using Datadog.AgentCustomActions;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Moq;
using System.Collections.Generic;
using System.Security.Principal;

namespace CustomActions.Tests.InstallState
{
    public class InstallStateTestSetup : SessionTestBaseSetup
    {
        public Fixture Fixture { get; } = new();

        public Mock<IRegistryServices> RegistryServices { get; } = new();
        public Mock<IServiceController> ServiceController { get; } = new();

        public Mock<INativeMethods> NativeMethods { get; } = new();

        public InstallStateTestSetup()
        {
            ServiceController.SetupGet(s => s.Services).Returns(new WindowsService[] { });
        }

        public ReadInstallStateCA Create()
        {
            return new ReadInstallStateCA(
                Session.Object,
                RegistryServices.Object,
                ServiceController.Object,
                NativeMethods.Object);
        }

        public InstallStateTestSetup WithRegistryKey(Registries registry, string path, Dictionary<string, object> keys)
        {
            var mockRegKey = Fixture.Create<Mock<IRegistryKey>>();
            RegistryServices.Setup(
                r => r.OpenRegistryKey(registry, path)).Returns(mockRegKey.Object);
            foreach (var kvp in keys)
            {
                mockRegKey.Setup(r => r.GetValue(kvp.Key)).Returns(kvp.Value);
            }

            return this;
        }

        public InstallStateTestSetup WithAgentServiceAccount(string account)
        {
            var serviceKey = new Mock<IRegistryKey>();
            serviceKey.Setup(r => r.GetValue("ObjectName")).Returns(account);
            RegistryServices.Setup(r => r.OpenRegistryKey(Registries.LocalMachine,
                $@"SYSTEM\CurrentControlSet\Services\{Constants.AgentServiceName}")).Returns(serviceKey.Object);
            return this;
        }

        /// <summary>
        /// Resolves <c>accountName</c> to <c>domain\user</c> like LookupAccountName.
        /// </summary>
        public InstallStateTestSetup WithAccount(string accountName, string domain, string user,
            SecurityIdentifier accountSid = null, SID_NAME_USE accountNameUse = SID_NAME_USE.SidTypeUser)
        {
            NativeMethods.Setup(n => n.LookupAccountName(
                    accountName,
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(new LookupAccountNameDelegate(
                    (
                        string _,
                        out string u,
                        out string d,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse
                    ) =>
                    {
                        u = user;
                        d = domain;
                        sid = accountSid;
                        nameUse = accountNameUse;
                    }))
                .Returns(true);

            return this;
        }
    }
}
