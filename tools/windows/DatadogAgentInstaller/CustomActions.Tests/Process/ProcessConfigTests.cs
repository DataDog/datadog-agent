using AutoFixture.Xunit2;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Moq;
using Xunit;

namespace CustomActions.Tests.Process
{
    /// <summary>
    /// Process specific tests.
    /// </summary>
    public class ProcessConfigTests
    {
        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_When_Process_Enabled(
            Mock<ISession> sessionMock,
            string processEnabled)
        {
            var datadogYaml = @"
# process_config:

  # process_collection:
    # enabled: false

  # container_collection:
    # enabled: true

  ## Deprecated - use `process_collection.enabled` and `container_collection.enabled` instead
  # enabled: ""true""
";
            sessionMock.Setup(session => session["PROCESS_ENABLED"]).Returns(processEnabled);
            var resultingYaml = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object).ToYaml();
            resultingYaml
                .Should()
                .HaveKey("process_config.process_collection.enabled")
                .And.HaveValue(processEnabled);
            resultingYaml
                .Should()
                .NotHaveKey("process_config.enabled");
        }

        [Theory]
        [InlineAutoData]
        public void Should_Always_Set_process_dd_url(
            Mock<ISession> sessionMock,
            string processDdUrl)
        {
            var datadogYaml = @"
# process_config:

  # process_collection:
    # enabled: false

  # container_collection:
    # enabled: true

  ## Deprecated - use `process_collection.enabled` and `container_collection.enabled` instead
  # enabled: ""true""
";
            sessionMock.Setup(session => session["PROCESS_DD_URL"]).Returns(processDdUrl);
            var r = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object);
            var resultingYaml = r.ToYaml();
            resultingYaml
                .Should()
                .HaveKey("process_config.process_dd_url")
                .And.HaveValue(processDdUrl);
        }

        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_When_Process_Url_Set_And_Process_Enabled(
            Mock<ISession> sessionMock,
            string processEnabled,
            string processDdUrl)
        {
            var datadogYaml = @"
# process_config:

  ## @param process_collection - custom object - optional
  ## Specifies settings for collecting processes.
  # process_collection:
    ## @param enabled - boolean - optional - default: false
    ## Enables collection of information about running processes.
    # enabled: false

  ## @param container_collection - custom object - optional
  ## Specifies settings for collecting containers.
  # container_collection:
    ## @param enabled - boolean - optional - default: true
    ## Enables collection of information about running containers.
    # enabled: true

  ## Deprecated - use `process_collection.enabled` and `container_collection.enabled` instead
  ## @param enabled - string - optional - default: ""false""
  ## @env DD_PROCESS_CONFIG_ENABLED - string - optional - default: ""false""
  ##  A string indicating the enabled state of the Process Agent:
  ##    * ""false""    : The Agent collects only containers information.
  ##    * ""true""     : The Agent collects containers and processes information.
  ##    * ""disabled"" : The Agent process collection is disabled.
  #
  # enabled: ""true""
";
            sessionMock.Setup(session => session["PROCESS_ENABLED"]).Returns(processEnabled);
            sessionMock.Setup(session => session["PROCESS_DD_URL"]).Returns(processDdUrl);
            var resultingYaml = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object).ToYaml();
            resultingYaml
                .Should()
                .HaveKey("process_config.process_collection.enabled")
                .And.HaveValue(processEnabled);
            resultingYaml
                .Should()
                .HaveKey("process_config.process_dd_url")
                .And.HaveValue(processDdUrl);
        }

        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_When_Process_Discovery_Enabled(
            Mock<ISession> sessionMock,
            string processDiscovery)
        {
            var datadogYaml = @"
# process_config:

  # enabled: ""disabled""

  # process_discovery:
    # enabled: false
";
            sessionMock.Setup(session => session["PROCESS_DISCOVERY_ENABLED"]).Returns(processDiscovery);
            var resultingYaml = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object).ToYaml();
            resultingYaml
                .Should()
                .HaveKey("process_config.process_discovery.enabled")
                .And.HaveValue(processDiscovery);
        }
    }
}
