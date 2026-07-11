using System;
using System.IO;
using System.Reflection;
using AutoFixture.Xunit2;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using Moq;
using WixToolset.Dtf.WindowsInstaller;
using Xunit;
using YamlDotNet.RepresentationModel;
using ISession = Datadog.CustomActions.Interfaces.ISession;

namespace CustomActions.Tests
{
    public class WriteConfigUnitTests
    {
        private const string CustomChromeExtensionId = "abcdefghijklmnopabcdefghijklmnop";

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

        [Theory]
        [InlineAutoData]
        public void WriteConfig_Should_Generate_AiUsageNativeHostConfig_With_Default_Apm_Port(Mock<ISession> sessionMock)
        {
            WithTempInstallFolders((configFolder, projectLocation) =>
            {
                File.WriteAllText(Path.Combine(configFolder, "datadog.yaml.example"), "api_key:\n");
                WriteAiUsageNativeHostExample(configFolder);
                Directory.CreateDirectory(AiUsageManifestDir(projectLocation));
                File.WriteAllText(Path.Combine(AiUsageManifestDir(projectLocation), "com.datadoghq.ai_prompt_logger.native_host.json"), "{}");
                sessionMock.Setup(session => session["APPLICATIONDATADIRECTORY"]).Returns(configFolder);
                sessionMock.Setup(session => session["PROJECTLOCATION"]).Returns(projectLocation);

                var result = InvokeWriteConfig(sessionMock.Object);

                Assert.Equal(ActionResult.Success, result);
                var aiUsageYaml = File.ReadAllText(Path.Combine(configFolder, "ai_usage_native_host.yaml"));
                Assert.Contains("trace_agent_url: \"http://127.0.0.1:8126\"", aiUsageYaml);
                Assert.DoesNotContain("chrome_extension_id:", aiUsageYaml);

                var manifest = File.ReadAllText(AiUsageManifestPath(projectLocation));
                Assert.Contains("\"name\": \"com.datadoghq.ai_usage_agent.native_host\"", manifest);
                Assert.Contains("\"path\": \"" + Path.Combine(projectLocation, "bin", "agent", "ai-usage-agent-native-host.exe").Replace("\\", "\\\\") + "\"", manifest);
                Assert.Contains($"\"chrome-extension://{Constants.FallbackAiUsageChromeExtensionId}/\"", manifest);
                Assert.False(File.Exists(Path.Combine(AiUsageManifestDir(projectLocation), "com.datadoghq.ai_prompt_logger.native_host.json")));
            });
        }

        [Theory]
        [InlineAutoData]
        public void WriteConfig_Should_Generate_AiUsageNativeHostConfig_With_DatadogYaml_Apm_Port(Mock<ISession> sessionMock)
        {
            WithTempInstallFolders((configFolder, projectLocation) =>
            {
                File.WriteAllText(Path.Combine(configFolder, "datadog.yaml.example"), "api_key:\n");
                File.WriteAllText(
                    Path.Combine(configFolder, "datadog.yaml"),
                    "apm_config:\n  enabled: true\n  receiver_port: 8136\n");
                WriteAiUsageNativeHostExample(configFolder);
                sessionMock.Setup(session => session["APPLICATIONDATADIRECTORY"]).Returns(configFolder);
                sessionMock.Setup(session => session["PROJECTLOCATION"]).Returns(projectLocation);

                var result = InvokeWriteConfig(sessionMock.Object);

                Assert.Equal(ActionResult.Success, result);
                var aiUsageYaml = File.ReadAllText(Path.Combine(configFolder, "ai_usage_native_host.yaml"));
                Assert.Contains("trace_agent_url: \"http://127.0.0.1:8136\"", aiUsageYaml);
            });
        }

        [Theory]
        [InlineAutoData]
        public void WriteConfig_Should_Not_Overwrite_Existing_AiUsageNativeHostConfig(Mock<ISession> sessionMock)
        {
            WithTempInstallFolders((configFolder, projectLocation) =>
            {
                var existingAiUsageConfig = "trace_agent_url: \"http://127.0.0.1:9999\"\n" +
                                            $"chrome_extension_id: \"{CustomChromeExtensionId}\"\n";
                File.WriteAllText(Path.Combine(configFolder, "datadog.yaml.example"), "api_key:\n");
                File.WriteAllText(
                    Path.Combine(configFolder, "datadog.yaml"),
                    "apm_config:\n  receiver_port: 8136\n");
                WriteAiUsageNativeHostExample(configFolder);
                File.WriteAllText(Path.Combine(configFolder, "ai_usage_native_host.yaml"), existingAiUsageConfig);
                sessionMock.Setup(session => session["APPLICATIONDATADIRECTORY"]).Returns(configFolder);
                sessionMock.Setup(session => session["PROJECTLOCATION"]).Returns(projectLocation);

                var result = InvokeWriteConfig(sessionMock.Object);

                Assert.Equal(ActionResult.Success, result);
                Assert.Equal(existingAiUsageConfig, File.ReadAllText(Path.Combine(configFolder, "ai_usage_native_host.yaml")));
                var manifest = File.ReadAllText(AiUsageManifestPath(projectLocation));
                Assert.Contains($"\"chrome-extension://{CustomChromeExtensionId}/\"", manifest);
            });
        }

        [Theory]
        [InlineAutoData]
        public void WriteConfig_Should_Generate_AiUsageNativeHostManifest_With_Fallback_Extension_Id_When_Config_Is_Missing(Mock<ISession> sessionMock)
        {
            WithTempInstallFolders((configFolder, projectLocation) =>
            {
                File.WriteAllText(Path.Combine(configFolder, "datadog.yaml.example"), "api_key:\n");
                sessionMock.Setup(session => session["APPLICATIONDATADIRECTORY"]).Returns(configFolder);
                sessionMock.Setup(session => session["PROJECTLOCATION"]).Returns(projectLocation);

                var result = InvokeWriteConfig(sessionMock.Object);

                Assert.Equal(ActionResult.Success, result);
                var manifest = File.ReadAllText(AiUsageManifestPath(projectLocation));
                Assert.Contains($"\"chrome-extension://{Constants.FallbackAiUsageChromeExtensionId}/\"", manifest);
                sessionMock.Verify(
                    session => session.Log(
                        It.Is<string>(message => message.Contains("using fallback Chrome extension ID")),
                        It.IsAny<string>(),
                        It.IsAny<string>(),
                        It.IsAny<int>()),
                    Times.Once);
            });
        }

        private static ActionResult InvokeWriteConfig(ISession session)
        {
            var method = typeof(ConfigCustomActions).GetMethod(
                "WriteConfig",
                BindingFlags.NonPublic | BindingFlags.Static,
                null,
                new[] { typeof(ISession) },
                null);
            return (ActionResult)method.Invoke(null, new object[] { session });
        }

        private static void WithTempInstallFolders(Action<string, string> action)
        {
            var tempRoot = Path.Combine(Path.GetTempPath(), Path.GetRandomFileName());
            var configFolder = Path.Combine(tempRoot, "programdata", "Datadog");
            var projectLocation = Path.Combine(tempRoot, "programfiles", "Datadog Agent");
            Directory.CreateDirectory(configFolder);
            Directory.CreateDirectory(projectLocation);
            try
            {
                action(configFolder, projectLocation);
            }
            finally
            {
                Directory.Delete(tempRoot, recursive: true);
            }
        }

        private static string AiUsageManifestPath(string projectLocation)
        {
            return Path.Combine(projectLocation, "bin", "agent", "dist", "com.datadoghq.ai_usage_agent.native_host.json");
        }

        private static string AiUsageManifestDir(string projectLocation)
        {
            return Path.Combine(projectLocation, "bin", "agent", "dist");
        }

        private static void WriteAiUsageNativeHostExample(string configFolder)
        {
            File.WriteAllText(
                Path.Combine(configFolder, "ai_usage_native_host.yaml.example"),
                "trace_agent_url: \"http://127.0.0.1:8126\"\n" +
                "evp_proxy_api_version: 2\n" +
                "logs_evp_subdomain: \"http-intake.logs\"\n");
        }
    }
}
