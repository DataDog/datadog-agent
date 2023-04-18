using System.ServiceProcess;
using AutoFixture.Xunit2;
using Datadog.CustomActions;
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
                .NotContainKeys(
                    "DDAGENTUSER_NAME",
                    "PROJECTLOCATION",
                    "APPLICATIONDATADIRECTORY",
                    "CHECKBOX_ALLOWCLOSEDSOURCE",
                    "WindowsBuild");

            // Must always be set in order to be written to the registry
            Test.Properties.Should()
                .Contain("ALLOWCLOSEDSOURCE", Constants.AllowClosedSource_No);
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
                    ["AllowClosedSource"] = Constants.AllowClosedSource_Yes,
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
                .Contain("ALLOWCLOSEDSOURCE", Constants.AllowClosedSource_Yes).And
                .Contain("CHECKBOX_ALLOWCLOSEDSOURCE", Constants.AllowClosedSource_Yes).And
                .Contain("WindowsBuild", "z_1234567890");
        }

        [Theory]
        [InlineAutoData(ServiceStartMode.Automatic, Constants.AllowClosedSource_Yes)]
        [InlineAutoData(ServiceStartMode.Boot, Constants.AllowClosedSource_Yes)]
        [InlineAutoData(ServiceStartMode.Manual, Constants.AllowClosedSource_Yes)]
        [InlineAutoData(ServiceStartMode.System, Constants.AllowClosedSource_Yes)]
        [InlineAutoData(ServiceStartMode.Disabled, Constants.AllowClosedSource_No)]
        public void ReadInstallState_Should_Read_Ddnpm_InstallState_If_AllowClosedSource_Missing(
            ServiceStartMode serviceStartMode,
            string exepctedAllowClosedSource)
        {
            Test.WithDdnpmService(serviceStartMode)
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("ALLOWCLOSEDSOURCE", exepctedAllowClosedSource);

            if (exepctedAllowClosedSource == Constants.AllowClosedSource_Yes)
            {
                Test.Properties.Should()
                    .Contain("CHECKBOX_ALLOWCLOSEDSOURCE", Constants.AllowClosedSource_Yes);
            }
            else
            {
                Test.Properties.Should()
                    .NotContainKey("CHECKBOX_ALLOWCLOSEDSOURCE");
            }
        }

        [Theory]
        [InlineData(Constants.AllowClosedSource_No, ServiceStartMode.Manual, Constants.AllowClosedSource_No)]
        [InlineData(Constants.AllowClosedSource_Yes, ServiceStartMode.Disabled, Constants.AllowClosedSource_Yes)]
        public void ReadInstallState_Should_AllowClosedSource_Ignore_Service_State_If_RegKey_Present(
            string allowClosedSource,
            ServiceStartMode serviceStartMode,
            string exepctedAllowClosedSource)
        {
            Test.WithRegistryKey(Registries.LocalMachine, @"Software\Datadog\Datadog Agent", new()
                {
                    ["AllowClosedSource"] = allowClosedSource,
                })
                .WithDdnpmService(serviceStartMode)
                .Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("ALLOWCLOSEDSOURCE", exepctedAllowClosedSource);

            if (exepctedAllowClosedSource == Constants.AllowClosedSource_Yes)
            {
                Test.Properties.Should()
                    .Contain("CHECKBOX_ALLOWCLOSEDSOURCE", Constants.AllowClosedSource_Yes);
            }
            else
            {
                Test.Properties.Should()
                    .NotContainKey("CHECKBOX_ALLOWCLOSEDSOURCE");
            }
        }
    }
}
