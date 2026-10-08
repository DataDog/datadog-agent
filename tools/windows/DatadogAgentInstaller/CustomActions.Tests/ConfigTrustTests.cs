using System;
using System.IO;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using FluentAssertions;
using WixToolset.Dtf.WindowsInstaller;
using Xunit;

namespace CustomActions.Tests
{
    public class ConfigTrustTests : SessionTestBaseSetup, IDisposable
    {
        private readonly string _configRoot = Path.Combine(Path.GetTempPath(), $"ConfigTrustTests-{Guid.NewGuid()}");

        public ConfigTrustTests()
        {
            Session.Object["APPLICATIONDATADIRECTORY"] = _configRoot;
        }

        public void Dispose()
        {
            if (Directory.Exists(_configRoot))
            {
                Directory.Delete(_configRoot, true);
            }
        }

        [Fact]
        public void Invalid_Root_Skips_Import()
        {
            Session.Object[PrerequisitesCustomActions.ConfigRootUntrustedProperty] = "False";
            Session.Object["APPLICATIONDATADIRECTORY"] = "";

            ConfigCustomActions.ReadConfig(Session.Object).Should().Be(ActionResult.Success);

            Session.Object[PrerequisitesCustomActions.ConfigRootUntrustedProperty].Should().Be("True");
            Session.Object["DATADOGYAMLEXISTS"].Should().Be("no");
        }

        [Theory]
        [InlineData(false)]
        [InlineData(true)]
        public void UI_Check_Reports_Validation_Without_Exiting(bool invalid)
        {
            if (invalid)
            {
                Session.Object["APPLICATIONDATADIRECTORY"] = "";
            }

            PrerequisitesCustomActions.EnsureSecureConfigRoot(Session.Object,
                calledFromUIControl: true).Should().Be(ActionResult.Success);

            Session.Object[PrerequisitesCustomActions.ConfigRootUntrustedProperty].Should().Be(invalid ? "True" : "False");
            if (invalid)
            {
                Session.Object["ErrorModal_ErrorMessage"].Should().NotBeNullOrEmpty();
            }
        }

        [Fact]
        public void Execution_Check_Fails_When_Root_Is_Invalid()
        {
            Session.Object["APPLICATIONDATADIRECTORY"] = "";

            PrerequisitesCustomActions.EnsureSecureConfigRoot(Session.Object).Should().Be(ActionResult.Failure);

            Session.Object[PrerequisitesCustomActions.ConfigRootUntrustedProperty].Should().Be("True");
        }

        [ElevatedFact]
        public void Execution_Check_Rejects_Ownership_Changed_After_Config_Import()
        {
            TestDirectory.CreateOwnedBy(_configRoot, TestDirectory.Administrators);
            File.WriteAllText(Path.Combine(_configRoot, "datadog.yaml"), "site: custom.example\n");
            ConfigCustomActions.ReadConfig(Session.Object).Should().Be(ActionResult.Success);
            TestDirectory.SetOwner(_configRoot, TestDirectory.UntrustedOwner);

            PrerequisitesCustomActions.EnsureSecureConfigRoot(Session.Object).Should().Be(ActionResult.Failure);

            Session.Object[PrerequisitesCustomActions.ConfigRootUntrustedProperty].Should().Be("True");
        }

        [Fact]
        public void Missing_Root_Is_Not_Created_Or_Read()
        {
            ConfigCustomActions.ReadConfig(Session.Object).Should().Be(ActionResult.Success);

            Directory.Exists(_configRoot).Should().BeFalse("the check must remain read-only");
            Session.Object[PrerequisitesCustomActions.ConfigRootUntrustedProperty].Should().Be("False");
            Session.Object["DATADOGYAMLEXISTS"].Should().Be("no");
        }

        [ElevatedTheory]
        [InlineData(false)]
        [InlineData(true)]
        public void Config_Import_Rechecks_Ownership_Regardless_Of_Cached_Trust(bool trusted)
        {
            TestDirectory.CreateOwnedBy(_configRoot,
                trusted ? TestDirectory.Administrators : TestDirectory.UntrustedOwner);
            File.WriteAllText(Path.Combine(_configRoot, "datadog.yaml"), "site: custom.example\n");
            Session.Object["APIKEY"] = "administrator-key";
            Session.Object[PrerequisitesCustomActions.ConfigRootUntrustedProperty] = trusted ? "True" : "False";

            ConfigCustomActions.ReadConfig(Session.Object).Should().Be(ActionResult.Success);

            Session.Object["SITE"].Should().Be(trusted ? "custom.example" : null);
            Session.Object["APIKEY"].Should().Be("administrator-key");
            Session.Object["DATADOGYAMLEXISTS"].Should().Be(trusted ? "yes" : "no");
            Session.Object[PrerequisitesCustomActions.ConfigRootUntrustedProperty].Should().Be(trusted ? "False" : "True");
        }
    }
}
