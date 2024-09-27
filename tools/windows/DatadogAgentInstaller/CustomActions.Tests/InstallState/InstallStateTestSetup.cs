using AutoFixture;
using Datadog.AgentCustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Moq;
using System.Collections.Generic;

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
    }
}
