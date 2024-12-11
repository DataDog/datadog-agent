using System.Collections.Generic;
using System.IO;
using AutoFixture.Xunit2;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Moq;
using Xunit;

namespace CustomActions.Tests.IntegrationTests
{
    /// <summary>
    /// Kitchen-sink tests.
    /// </summary>
    /// These tests expects a valid config to be found in the same folder as where the unit
    /// tests run. The CI takes care of generating a valid configuration that will be placed
    /// at the right location using the agent.generate-config invoke task.
    public class TestConfig
    {
        /// <summary>
        /// Base on win-installopts kitchen test
        /// </summary>
        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_Properties_For_win_installopts(Mock<ISession> sessionMock)
        {
            foreach (var keyValuePair in new Dictionary<string, string>()
                     {
                         { "APIKEY", "testapikey" },
                         { "TAGS", "k1:v1,k2:v2" },
                         { "CMD_PORT", "4999" },
                         { "PROXY_HOST", "proxy.foo.com" },
                         { "PROXY_PORT", "1234" },
                         { "PROXY_USER", "puser" },
                         { "PROXY_PASSWORD", "ppass" },
                         { "SITE", "eu" },
                         { "DD_URL", "https://someurl.datadoghq.com" },
                         { "LOGS_DD_URL", "https://logs.someurl.datadoghq.com" },
                         { "PROCESS_DD_URL", "https://process.someurl.datadoghq.com" },
                         { "TRACE_DD_URL", "https://trace.someurl.datadoghq.com" },
                     })
            {
                sessionMock.Setup(session => session[keyValuePair.Key]).Returns(keyValuePair.Value);
            }

            var datadogYaml = File.ReadAllText(@"datadog.yaml");
            var newConfig = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object);
            var resultingYaml = newConfig.ToYaml();
            resultingYaml
                .Should()
                .HaveKey("api_key")
                .And.HaveValue("testapikey");
            resultingYaml
                .Should()
                .HaveKey("tags")
                .And.HaveValues(new[]
                {
                    "k1:v1",
                    "k2:v2"
                });
            resultingYaml
                .Should()
                .HaveKey("cmd_port")
                .And.HaveValue("4999");
            resultingYaml
                .Should()
                .HaveKey("proxy.http")
                .And.HaveValue("http://puser:ppass@proxy.foo.com:1234/");
            resultingYaml
                .Should()
                .HaveKey("proxy.https")
                .And.HaveValue("http://puser:ppass@proxy.foo.com:1234/");
            resultingYaml
                .Should()
                .HaveKey("site")
                .And.HaveValue("eu");
            resultingYaml
                .Should()
                .HaveKey("dd_url")
                .And.HaveValue("https://someurl.datadoghq.com");
            resultingYaml
                .Should()
                .HaveKey("logs_config.logs_dd_url")
                .And.HaveValue("https://logs.someurl.datadoghq.com");
            resultingYaml
                .Should()
                .HaveKey("process_config.process_dd_url")
                .And.HaveValue("https://process.someurl.datadoghq.com");
            resultingYaml
                .Should()
                .HaveKey("apm_config.apm_dd_url")
                .And.HaveValue("https://trace.someurl.datadoghq.com");
        }

        /// <summary>
        /// Base on win-all-subservices kitchen test
        /// </summary>
        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_Properties_For_win_all_subservices(Mock<ISession> sessionMock)
        {
            foreach (var keyValuePair in new Dictionary<string, string>()
                     {
                         { "APIKEY", "testapikey" },
                         { "LOGS_ENABLED", "true" },
                         { "PROCESS_ENABLED", "true" },
                         { "APM_ENABLED", "true" },
                     })
            {
                sessionMock.Setup(session => session[keyValuePair.Key]).Returns(keyValuePair.Value);
            }

            var datadogYaml = File.ReadAllText(@"datadog.yaml");
            var newConfig = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object);
            var resultingYaml = newConfig.ToYaml();
            resultingYaml
                .Should()
                .HaveKey("api_key")
                .And.HaveValue("testapikey");
            resultingYaml.HasLogsEnabled();
            resultingYaml.HasProcessEnabled();
            resultingYaml.HasApmEnabled();
        }
    }
}
