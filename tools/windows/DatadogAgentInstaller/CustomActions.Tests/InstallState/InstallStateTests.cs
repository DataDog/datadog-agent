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
                    "DDAGENT_installedDomain",
                    "DDAgent_installedUser",
                    "PROJECTLOCATION",
                    "APPLICATIONDATADIRECTORY",
                    "DDAGENT_WINDOWSBUILD").And
                .Contain("DDDRIVERROLLBACK_NPM", "1").And
                .Contain("DDDRIVERROLLBACK_PROCMON", "1");
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
                ["ConfigRoot"] = @"D:\data"
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
                .Contain("DDAGENT_installedDomain", "testDomain").And
                .Contain("DDAGENT_installedUser", "testUser").And
                .Contain("PROJECTLOCATION", @"C:\datadog").And
                .Contain("APPLICATIONDATADIRECTORY", @"D:\data").And
                .Contain("DDAGENT_WINDOWSBUILD", "z_1234567890");
        }

        [Theory]
        [InlineData("7.53", "", "")]
        [InlineData("6.52", "", "")]
        [InlineData("7.51", "", "1")]
        [InlineData("6.58", "1", "1")]
        [InlineData("7.56", "1", "1")]
        public void ReadDD_Driver_Rollback_Upgrade(string version, string NPMExpectedRollback, string procmonExopectedRollback)
        {
            var productCode = "{123-456-789}";
            Test.Session.Setup(session => session["WIX_UPGRADE_DETECTED"]).Returns(productCode);
            Test.NativeMethods.Setup(n => n.GetVersionString(productCode)).Returns(version);

            Test.Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDDRIVERROLLBACK_NPM", NPMExpectedRollback).And
                .Contain("DDDRIVERROLLBACK_PROCMON", procmonExopectedRollback);
        }

        // This test covers the scenario where we fail to read the version we are upgrading from.
        // This shouldn't normally happen, we've seen the following cases:
        // - WIX_UGPRADE_DETECTED contains multiple product codes
        // - GetVersionString failes with "Unknown property"
        [Theory]
        [AutoData]
        public void ReadDD_Driver_Rollback_Multiple_Product_Codes()
        {
            var productCode = "{123-456-789};{123-456-790}";
            Test.Session.Setup(session => session["WIX_UPGRADE_DETECTED"]).Returns(productCode);
            Test.NativeMethods.Setup(n => n.GetVersionString(productCode)).Throws(new System.Exception("GetVersionString failed"));

            Test.Create()
                .ReadInstallState()
                .Should()
                .Be(ActionResult.Success);

            Test.Properties.Should()
                .Contain("DDDRIVERROLLBACK_NPM", "1").And
                .Contain("DDDRIVERROLLBACK_PROCMON", "1");
        }

    }
}
