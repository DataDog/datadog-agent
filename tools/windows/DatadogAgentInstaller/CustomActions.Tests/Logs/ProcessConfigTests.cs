using AutoFixture.Xunit2;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Moq;
using Xunit;

namespace CustomActions.Tests.Logs
{
    /// <summary>
    /// Logs specific tests.
    /// </summary>
    public class LogsConfigTests
    {
        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_When_Dd_Logs_Url_Specified(Mock<ISession> sessionMock, string ddLogsUrl)
        {
            var datadogYaml = @"
# logs_enabled: false

# logs_config:

  # logs_dd_url: <ENDPOINT>:<PORT>
";
            sessionMock.Setup(session => session["LOGS_DD_URL"]).Returns(ddLogsUrl);
            var resultingYaml = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object).ToYaml();
            resultingYaml
                .Should()
                .HaveKey("logs_config.logs_dd_url")
                .And.HaveValue(ddLogsUrl);
            resultingYaml
                .Should()
                .NotHaveKey("logs_enabled");
        }
    }
}
