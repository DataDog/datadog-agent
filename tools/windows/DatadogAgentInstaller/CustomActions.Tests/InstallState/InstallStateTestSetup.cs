using System.Collections.Generic;
using AutoFixture;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Moq;

namespace CustomActions.Tests.InstallState;

public class InstallStateTestSetup : SessionTestBaseSetup
{
    public Fixture Fixture { get; } = new();

    public Mock<IRegistryServices> RegistryServices { get; } = new();

    public InstallStateCustomActions Create()
    {
        return new InstallStateCustomActions(Session.Object, RegistryServices.Object);
    }

    public InstallStateTestSetup WithRegistryKey(Registries registry, string path, Dictionary<string, object> keys)
    {
        var mockRegKey = Fixture.Create<Mock<IRegistryKey>>();
        RegistryServices.Setup(
            r => r.OpenRegistryKey(Registries.LocalMachine, path)).Returns(mockRegKey.Object);
        foreach (var kvp in keys)
        {
            mockRegKey.Setup(r => r.GetValue(kvp.Key)).Returns(kvp.Value);
        }

        return this;
    }
}
