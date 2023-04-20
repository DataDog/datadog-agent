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
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
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
        [InlineAutoData(Constants.AllowClosedSource_No, Constants.AllowClosedSource_No)]
        [InlineAutoData(Constants.AllowClosedSource_Yes, Constants.AllowClosedSource_Yes)]
        public void ReadInstallState_CommandLine_Overrides_AllowClosedSource(
            string commandlineAllowClosedSource,
            string expectedAllowClosedSource)
        {
            var opposite = (commandlineAllowClosedSource == Constants.AllowClosedSource_Yes)
                ? Constants.AllowClosedSource_No
                : Constants.AllowClosedSource_Yes;
            Test.Session.Object["ALLOWCLOSEDSOURCE"] = commandlineAllowClosedSource;

            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    // Set the registry key to the opposite ensure it is overriden
                    ["AllowClosedSource"] = opposite,
                }).WithFeatureState(new()
                {
                    ["NPM"] = (Microsoft.Deployment.WindowsInstaller.InstallState.Local,
                        Microsoft.Deployment.WindowsInstaller.InstallState.Local),
                }).Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("ALLOWCLOSEDSOURCE", expectedAllowClosedSource);

            if (expectedAllowClosedSource == Constants.AllowClosedSource_Yes)
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
        [InlineAutoData(Microsoft.Deployment.WindowsInstaller.InstallState.Absent, Constants.AllowClosedSource_No,
            Constants.AllowClosedSource_No)]
        [InlineAutoData(Microsoft.Deployment.WindowsInstaller.InstallState.Absent, Constants.AllowClosedSource_Yes,
            Constants.AllowClosedSource_Yes)]
        [InlineAutoData(Microsoft.Deployment.WindowsInstaller.InstallState.Local, Constants.AllowClosedSource_No,
            Constants.AllowClosedSource_Yes)]
        [InlineAutoData(Microsoft.Deployment.WindowsInstaller.InstallState.Local, Constants.AllowClosedSource_Yes,
            Constants.AllowClosedSource_Yes)]
        public void ReadInstallState_NPM_FeatureState_Overrides_AllowClosedSource_Registry(
            Microsoft.Deployment.WindowsInstaller.InstallState NPMFeatureState,
            string registryAllowClosedSource,
            string expectedAllowClosedSource)
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    // Set the registry key to the opposite ensure it is overriden
                    ["AllowClosedSource"] = registryAllowClosedSource,
                }).WithFeatureState(new()
                {
                    ["NPM"] = (Microsoft.Deployment.WindowsInstaller.InstallState.Absent, NPMFeatureState),
                }).Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("ALLOWCLOSEDSOURCE", expectedAllowClosedSource);

            if (expectedAllowClosedSource == Constants.AllowClosedSource_Yes)
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
        [InlineAutoData(false, Constants.AllowClosedSource_No, Constants.AllowClosedSource_No)]
        [InlineAutoData(false, Constants.AllowClosedSource_Yes, Constants.AllowClosedSource_Yes)]
        [InlineAutoData(true, Constants.AllowClosedSource_No, Constants.AllowClosedSource_Yes)]
        [InlineAutoData(true, Constants.AllowClosedSource_Yes, Constants.AllowClosedSource_Yes)]
        public void ReadInstallState_NPM_Property_Overrides_AllowClosedSource_Registry(
            bool NPMFlag,
            string registryAllowClosedSource,
            string expectedAllowClosedSource)
        {
            if (NPMFlag)
            {
                Test.Session.Object["NPM"] = "somevalue";
            }

            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["AllowClosedSource"] = registryAllowClosedSource,
                }).Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("ALLOWCLOSEDSOURCE", expectedAllowClosedSource);

            if (expectedAllowClosedSource == Constants.AllowClosedSource_Yes)
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
