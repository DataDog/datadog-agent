using AutoFixture.Xunit2;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Moq;
using Xunit;

namespace CustomActions.Tests.APM
{
    /// <summary>
    /// APM Config specific tests
    /// </summary>
    public class ApmConfigTests
    {
        [Theory]
        [InlineAutoData]
        public void Should_Always_Set_apm_dd_url(Mock<ISession> sessionMock, string traceDdUrl)
        {
            var datadogYaml = @"
# apm_config:

  # enabled: false

  # apm_dd_url: <ENDPOINT>:<PORT>
";
            sessionMock.Setup(session => session["TRACE_DD_URL"]).Returns(traceDdUrl);
            var resultingYaml = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object)
                .ToYaml();
            resultingYaml
                .Should()
                .HaveKey("apm_config.apm_dd_url")
                .And.HaveValue(traceDdUrl);
            resultingYaml
                .Should()
                .NotHaveKey("apm_config.enabled");
        }

        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_When_Trace_Url_Set_And_Apm_Enabled(Mock<ISession> sessionMock, string traceDdUrl, string apmEnabled)
        {
            var datadogYaml = @"
# apm_config:

  # enabled: false

  # apm_dd_url: <ENDPOINT>:<PORT>
";
            sessionMock.Setup(session => session["TRACE_DD_URL"]).Returns(traceDdUrl);
            sessionMock.Setup(session => session["APM_ENABLED"]).Returns(apmEnabled);
            var r = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object);
            var resultingYaml = r.ToYaml();
            resultingYaml
                .Should()
                .HaveKey("apm_config.apm_dd_url")
                .And.HaveValue(traceDdUrl);
            resultingYaml
                .Should()
                .HaveKey("apm_config.enabled")
                .And.HaveValue(apmEnabled);
        }

        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_When_Trace_Url_Unset_And_Apm_Enabled_Is_True(Mock<ISession> sessionMock, string apmEnabled)
        {
            var datadogYaml = @"
# apm_config:

  # enabled: false

  # apm_dd_url: <ENDPOINT>:<PORT>
";
            sessionMock.Setup(session => session["APM_ENABLED"]).Returns(apmEnabled);
            var resultingYaml = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object).ToYaml();
            resultingYaml
                .Should()
                .NotHaveKey("apm_config.apm_dd_url");
            resultingYaml
                .Should()
                .HaveKey("apm_config.enabled")
                .And.HaveValue(apmEnabled);
        }
    }
}
