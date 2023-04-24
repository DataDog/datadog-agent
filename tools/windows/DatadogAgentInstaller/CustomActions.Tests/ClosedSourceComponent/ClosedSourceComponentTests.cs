using AutoFixture.Xunit2;
using Datadog.CustomActions;
using Datadog.CustomActions.Native;
using FluentAssertions;
using Microsoft.Deployment.WindowsInstaller;
using Xunit;
// Simplifies type declaration
using FeatureInstallState = Microsoft.Deployment.WindowsInstaller.InstallState;
// Simplifies type usage
using static Microsoft.Deployment.WindowsInstaller.InstallState;

namespace CustomActions.Tests.ClosedSourceComponent
{
    public class ClosedSourceComponentTests
    {
        public ClosedSourceComponentTestsSetup Test { get; } = new();

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
                    [Constants.AllowClosedSourceRegistryKey] = opposite,
                })
                .WithFeatureState(new()
                    {
                        ["NPM"] = (Local, Local),
                    }
                ).Create()
                .ProcessAllowClosedSource()
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
        [InlineAutoData(Absent, Constants.AllowClosedSource_No, Constants.AllowClosedSource_No)]
        [InlineAutoData(Absent, Constants.AllowClosedSource_Yes, Constants.AllowClosedSource_Yes)]
        [InlineAutoData(Local, Constants.AllowClosedSource_No, Constants.AllowClosedSource_Yes)]
        [InlineAutoData(Local, Constants.AllowClosedSource_Yes, Constants.AllowClosedSource_Yes)]
        public void ReadInstallState_NPM_FeatureState_Overrides_AllowClosedSource_Registry(
            FeatureInstallState npmFeatureState,
            string registryAllowClosedSource,
            string expectedAllowClosedSource)
        {
            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    // Set the registry key to the opposite ensure it is overriden
                    [Constants.AllowClosedSourceRegistryKey] = registryAllowClosedSource,
                })
                .WithFeatureState(new()
                {
                    ["NPM"] = (Absent, npmFeatureState),
                })
                .Create()
                .ProcessAllowClosedSource()
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
            bool npmFlag,
            string registryAllowClosedSource,
            string expectedAllowClosedSource)
        {
            if (npmFlag)
            {
                Test.Session.Object["NPM"] = "somevalue";
            }

            Test.WithRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey, new()
                {
                    ["AllowClosedSource"] = registryAllowClosedSource,
                })
                .Create()
                .ProcessAllowClosedSource()
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
