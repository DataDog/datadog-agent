using System;
using System.IO;
using System.Security.AccessControl;
using AutoFixture.Xunit2;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using Moq;
using Xunit;
using YamlDotNet.RepresentationModel;
using WixToolset.Dtf.WindowsInstaller;
using ISession = Datadog.CustomActions.Interfaces.ISession;

namespace CustomActions.Tests
{
    public class WriteConfigUnitTests
    {
        [Theory]
        [InlineData(null, false)]
        [InlineData(null, true)]
        [InlineData("", true)]
        [InlineData("customer scripts and credentials\n", true)]
        public void ParConfig_Should_Be_Initialized_Only_When_Absent(string existingContent, bool exampleExists)
        {
            var root = Path.Combine(Path.GetTempPath(), $"ParConfigTests-{Guid.NewGuid()}");
            var live = Path.Combine(root, "private-action-runner", "powershell-script-config.yaml");
            Directory.CreateDirectory(Path.GetDirectoryName(live));
            try
            {
                const string defaults = "packaged sample scripts\n";
                if (exampleExists)
                {
                    File.WriteAllText(live + ".example", defaults);
                }
                string originalDacl = null;
                if (existingContent != null)
                {
                    File.WriteAllText(live, existingContent);
                    var security = File.GetAccessControl(live, AccessControlSections.Access);
                    security.SetAccessRuleProtection(true, true);
                    File.SetAccessControl(live, security);
                    originalDacl = File.GetAccessControl(live, AccessControlSections.Access)
                        .GetSecurityDescriptorSddlForm(AccessControlSections.Access);
                    File.SetAttributes(live, FileAttributes.ReadOnly);
                }
                var session = new Mock<ISession>();
                session.Setup(s => s["APPLICATIONDATADIRECTORY"]).Returns(root);

                Assert.Equal(ActionResult.Success, ConfigCustomActions.WriteConfig(session.Object));
                if (existingContent != null)
                {
                    Assert.Equal(existingContent, File.ReadAllText(live));
                    Assert.Equal(originalDacl, File.GetAccessControl(live, AccessControlSections.Access)
                        .GetSecurityDescriptorSddlForm(AccessControlSections.Access));
                    Assert.True((File.GetAttributes(live) & FileAttributes.ReadOnly) != 0);
                }
                else if (exampleExists)
                {
                    Assert.Equal(defaults, File.ReadAllText(live));
                }
                else
                {
                    Assert.False(File.Exists(live));
                }
            }
            finally
            {
                if (File.Exists(live))
                {
                    File.SetAttributes(live, FileAttributes.Normal);
                }
                Directory.Delete(root, true);
            }
        }

        [Theory]
        [InlineAutoData("APIKEY", "api_key")]
        [InlineAutoData("SITE", "site")]
        [InlineAutoData("HOSTNAME", "hostname")]
        [InlineAutoData("LOGS_ENABLED", "logs_enabled")]
        [InlineAutoData("CMD_PORT", "cmd_port")]
        [InlineAutoData("DD_URL", "dd_url")]
        [InlineAutoData("PYVER", "python_version")]
        [InlineAutoData("HOSTNAME_FQDN_ENABLED", "hostname_fqdn")]
        [InlineAutoData("EC2_USE_WINDOWS_PREFIX_DETECTION", "ec2_use_windows_prefix_detection")]
        public void ScalarProperties_Should_Be_Replaced_Given_They_Match(string property, string key, string value, Mock<ISession> sessionMock)
        {
            var datadogYaml = $@"
# Some comments
# {key}:";
            sessionMock.Setup(session => session[property]).Returns(value);
            ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object)
                .ToYaml()
                .Should().HaveKey(key)
                         .And.BeOfType(typeof(YamlScalarNode))
                         .And.HaveValue(value);
        }

        [Theory]
        [InlineAutoData("APIKEY", "api_key")]
        [InlineAutoData("SITE", "site")]
        [InlineAutoData("HOSTNAME", "hostname")]
        [InlineAutoData("LOGS_ENABLED", "logs_enabled")]
        [InlineAutoData("LOGS_DD_URL", "logs_dd_url")]
        [InlineAutoData("PROCESS_ENABLED", "process_config")]
        [InlineAutoData("PROCESS_DD_URL", "process_config")]
        [InlineAutoData("PROCESS_DISCOVERY_ENABLED", "process_discovery")]
        [InlineAutoData("APM_ENABLED", "apm_config")]
        [InlineAutoData("TRACE_DD_URL", "apm_config")]
        [InlineAutoData("PROXY_HOST", "proxy")]
        [InlineAutoData("TAGS", "tags")]
        [InlineAutoData("CMD_PORT", "cmd_port")]
        [InlineAutoData("DD_URL", "dd_url")]
        [InlineAutoData("PYVER", "python_version")]
        [InlineAutoData("HOSTNAME_FQDN_ENABLED", "hostname_fqdn")]
        public void Properties_Should_Not_Be_Replaced_Given_A_Property_Does_Not_Match(string property, string key, string value, Mock<ISession> sessionMock)
        {
            var datadogYaml = $@"
# This is a random yaml document.
# Define a single property so that the YAML loader doesn't
# consider the document empty.
random_property: test
";
            sessionMock.Setup(session => session[property]).Returns(value);

            ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object)
                .ToYaml()
                .Should()
                .NotHaveKey(key);
        }

        [Theory]
        [InlineAutoData("EC2_USE_WINDOWS_PREFIX_DETECTION", "ec2_use_windows_prefix_detection")]
        public void Missing_Properties_Should_Be_Appended(string property, string key, string value, Mock<ISession> sessionMock)
        {
            var datadogYaml = "";
            sessionMock.Setup(session => session[property]).Returns(value);
            ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object)
                .ToYaml()
                .Should()
                .HaveKey(key)
                .And.HaveValue(value);
        }
    }
}
