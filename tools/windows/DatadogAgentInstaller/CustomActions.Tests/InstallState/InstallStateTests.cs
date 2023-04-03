using System.ServiceProcess;
using AutoFixture.Xunit2;
using Datadog.CustomActions.Native;
using FluentAssertions;
using Microsoft.Deployment.WindowsInstaller;
using Xunit;

namespace CustomActions.Tests.InstallState
{
    public class InstallStateTests
    {
        public InstallStateTestSetup Test { get; } = new();

        [Theory]
        [AutoData]
        public void ReadInstallState_Default_Values()
        {
            Test.Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .NotContain(
                    "DDAGENTUSER_NAME",
                    "PROJECTLOCATION",
                    "APPLICATIONDATADIRECTORY",
                    "ALLOWCLOSEDSOURCE",
                    "WindowsBuild");
        }

        [Theory]
        [AutoData]
        public void ReadInstallState_Can_Read_Registry_Keys()
        {
            Test.WithRegistryKey(Registries.LocalMachine, @"Software\Datadog\Datadog Agent", new()
                {
                    ["installedDomain"] = "testDomain",
                    ["installedUser"] = "testUser",
                    ["InstallPath"] = @"C:\datadog",
                    ["ConfigRoot"] = @"D:\data",
                    ["AllowClosedSource"] = "xyz",
                })
                .WithRegistryKey(Registries.LocalMachine, @"Software\Microsoft\Windows NT\CurrentVersion", new()
                {
                    ["CurrentBuild"] = "z_1234567890",
                })
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDAGENTUSER_NAME", @"testDomain\testUser").And
                .Contain("PROJECTLOCATION", @"C:\datadog").And
                .Contain("APPLICATIONDATADIRECTORY", @"D:\data").And
                .Contain("ALLOWCLOSEDSOURCE", "xyz").And
                .Contain("WindowsBuild", "z_1234567890");
        }

        [Theory]
        [InlineAutoData(ServiceStartMode.Automatic)]
        [InlineAutoData(ServiceStartMode.Boot)]
        [InlineAutoData(ServiceStartMode.Manual)]
        [InlineAutoData(ServiceStartMode.System)]
        public void ReadInstallState_Should_Read_Ddnpm_InstallState_If_AllowClosedSource_Missing(ServiceStartMode serviceStartMode)
        {
            Test.WithRegistryKey(Registries.LocalMachine, @"SYSTEM\CurrentControlSet\Services\ddnpm", new()
                {
                    ["Start"] = serviceStartMode,
                })
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("ALLOWCLOSEDSOURCE", "1");
        }

        [Theory]
        [AutoData]
        public void ReadInstallState_Should_AllowClosedSource_Be_0_If_Ddnpm_Service_Disabled()
        {
            Test.WithRegistryKey(Registries.LocalMachine, @"SYSTEM\CurrentControlSet\Services\ddnpm", new()
                {
                    ["Start"] = ServiceStartMode.Disabled,
                })
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("ALLOWCLOSEDSOURCE", "0");
        }

        [Theory]
        [AutoData]
        public void ReadInstallState_Should_AllowClosedSource_Be_0_If_Ddnpm_Service_Has_Invalid_Value()
        {
            Test.WithRegistryKey(Registries.LocalMachine, @"SYSTEM\CurrentControlSet\Services\ddnpm", new()
                {
                    ["Start"] = "zzyy",
                })
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .NotContainKey("ALLOWCLOSEDSOURCE");
        }

        [Theory]
        [InlineData("0", ServiceStartMode.Manual, "0")]
        [InlineData("1", ServiceStartMode.Disabled, "1")]
        public void ReadInstallState_Should_AllowClosedSource_Ignore_Service_State_If_RegKey_Present(
            string allowClosedSource,
            ServiceStartMode serviceStartMode,
            string exepctedAllowClosedSource)
        {
            Test.WithRegistryKey(Registries.LocalMachine, @"Software\Datadog\Datadog Agent", new()
                {
                    ["AllowClosedSource"] = allowClosedSource,
                })
                .WithRegistryKey(Registries.LocalMachine, @"SYSTEM\CurrentControlSet\Services\ddnpm", new()
                {
                    ["Start"] = serviceStartMode,
                })
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("ALLOWCLOSEDSOURCE", exepctedAllowClosedSource);
        }
    }
}
