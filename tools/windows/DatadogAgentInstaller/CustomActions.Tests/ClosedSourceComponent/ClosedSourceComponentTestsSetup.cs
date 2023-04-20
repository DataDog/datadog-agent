using System.Collections.Generic;
using System.ServiceProcess;
using AutoFixture;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Moq;

namespace CustomActions.Tests.ClosedSourceComponent
{
    public class ClosedSourceComponentTestsSetup : SessionTestBaseSetup
    {
        public Fixture Fixture { get; } = new();

        public Mock<IRegistryServices> RegistryServices { get; } = new();
        public Mock<IServiceController> ServiceController { get; } = new();

        public ClosedSourceComponentTestsSetup()
        {
            ServiceController.SetupGet(s => s.Services).Returns(new WindowsService[] { });
            // default feature state
            WithFeatureState(new()
            {
                ["NPM"] = (Microsoft.Deployment.WindowsInstaller.InstallState.Absent,
                    Microsoft.Deployment.WindowsInstaller.InstallState.Absent),
            });
        }

        public ClosedSourceComponentsCustomActions Create()
        {
            return new ClosedSourceComponentsCustomActions(
                Session.Object,
                RegistryServices.Object);
        }

        public ClosedSourceComponentTestsSetup WithRegistryKey(Registries registry, string path, Dictionary<string, object> keys)
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

        public ClosedSourceComponentTestsSetup WithFeatureState(
            Dictionary<string, (Microsoft.Deployment.WindowsInstaller.InstallState,
                Microsoft.Deployment.WindowsInstaller.InstallState)> keys)
        {
            foreach (var kvp in keys)
            {
                var mockFeature = new Mock<IFeatureInfo>();
                Session.Setup(r => r.Feature(kvp.Key)).Returns(mockFeature.Object);
                mockFeature.Setup(r => r.CurrentState).Returns(kvp.Value.Item1);
                mockFeature.Setup(r => r.RequestState).Returns(kvp.Value.Item2);
            }

            return this;
        }

        public ClosedSourceComponentTestsSetup WithDdnpmService(ServiceStartMode? serviceStartMode = null)
        {
            var service = new Mock<IWindowsService>();
            service.SetupGet(s => s.DisplayName).Returns("Datadog NPM service");
            service.SetupGet(s => s.ServiceName).Returns(Constants.NpmServiceName);
            if (serviceStartMode != null)
            {
                service.SetupGet(s => s.StartType).Returns(serviceStartMode.Value);
            }

            ServiceController.SetupGet(s => s.Services).Returns(new[]
            {
                service.Object
            });

            return this;
        }
    }
}
