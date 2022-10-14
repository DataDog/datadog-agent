using AutoFixture.Xunit2;
using Datadog.CustomActions;
using Moq;
using Xunit;
using YamlDotNet.RepresentationModel;
using ISession = Datadog.CustomActions.ISession;

namespace CustomActions.Tests
{
    public class WriteConfigUnitTests
    {
        //[InlineAutoData("PROCESS_ENABLED", "process_config")]
        //[InlineAutoData("PROCESS_DD_URL", "process_config")]
        //[InlineAutoData("PROCESS_DISCOVERY_ENABLED", "process_discovery")]
        //[InlineAutoData("APM_ENABLED", "apm_config")]
        //[InlineAutoData("TRACE_DD_URL", "apm_config")]
        //[InlineAutoData("PROXY_HOST", "proxy")]
        //[InlineAutoData("TAGS", "tags", "tag1=someValue,tag2=other_value")]

        [Theory]
        [InlineAutoData("APIKEY", "api_key")]
        [InlineAutoData("SITE", "site")]
        [InlineAutoData("HOSTNAME", "hostname")]
        [InlineAutoData("LOGS_ENABLED", "logs_enabled")]
        [InlineAutoData("LOGS_DD_URL", "logs_dd_url")]
        [InlineAutoData("CMD_PORT", "cmd_port")]
        [InlineAutoData("DD_URL", "dd_url")]
        [InlineAutoData("PYVER", "python_version")]
        [InlineAutoData("HOSTNAME_FQDN_ENABLED", "hostname_fqdn")]
        [InlineAutoData("EC2_USE_WINDOWS_PREFIX_DETECTION", "ec2_use_windows_prefix_detection")]
        public void ScalarProperties_ShouldBeReplaced_GivenTheyMatch(string property, string key, string value)
        {
            var datadogYaml = $@"
# Some comments
# {key}:";
            var sessionMock = new Mock<ISession>();
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
        [InlineAutoData("EC2_USE_WINDOWS_PREFIX_DETECTION", "ec2_use_windows_prefix_detection")]
        public void Properties_ShouldNotBeReplaced_GivenAPropertyDoesNotMatch(string property, string key, string value)
        {
            var datadogYaml = $@"
# This is a random yaml document.
# Define a single property so that the YAML loader doesn't
# consider the document empty.
random_property: test
";
            var sessionMock = new Mock<ISession>();
            sessionMock.Setup(session => session[property]).Returns(value);

            ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object)
                .ToYaml()
                .Should()
                .NotHaveKey(key);
        }
    }
}
