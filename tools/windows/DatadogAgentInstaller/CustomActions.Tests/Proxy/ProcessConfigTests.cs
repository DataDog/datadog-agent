using AutoFixture.Xunit2;
using CustomActions.Tests.Helpers;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Moq;
using Xunit;

namespace CustomActions.Tests.Proxy
{
    /// <summary>
    /// Proxy specific tests.
    /// </summary>
    public class ProxyTests
    {
        /// <summary>
        /// Verifies that the replacer doesn't do anything when PROXY_HOST is missing.
        /// </summary>
        /// <param name="sessionMock">The mocked session.</param>
        /// <param name="proxyPort">The generated proxy port.</param>
        /// <param name="proxyUser">The generated proxy user.</param>
        /// <param name="proxyPassword">The generated proxy password.</param>
        [Theory]
        [InlineAutoData]
        public void Dont_Do_Anything_When_Only_Optional_Proxy_Values_Present(
            Mock<ISession> sessionMock,
            int proxyPort,
            string proxyUser,
            string proxyPassword)
        {
            var datadogYaml = @"
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>
";
            sessionMock.Setup(session => session["PROXY_PORT"]).Returns(proxyPort.ToString);
            sessionMock.Setup(session => session["PROXY_USER"]).Returns(proxyUser);
            sessionMock.Setup(session => session["PROXY_PASSWORD"]).Returns(proxyPassword);
            var resultingYaml = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object).ToYaml();
            resultingYaml
                .Should()
                .NotHaveKey("proxy.https");
            resultingYaml
                .Should()
                .NotHaveKey("proxy.https");
            resultingYaml
                .Should()
                .NotHaveKey("proxy.no_proxy");
        }

        /// <summary>
        /// Verifies that the replacer will insert the correct default values.
        /// </summary>
        /// <param name="sessionMock">The mocked session.</param>
        /// <param name="proxyHost">The generated proxy host.</param>
        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_When_Only_PROXY_HOST_Present(
            Mock<ISession> sessionMock,
            string proxyHost)
        {
            var datadogYaml = @"
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>
";
            sessionMock.Setup(session => session["PROXY_HOST"]).Returns(proxyHost);
            var r = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object);
            var resultingYaml = r.ToYaml();
            resultingYaml
                .Should()
                .HaveKey("proxy.https")
                .And.HaveValue($"http://{proxyHost.ToLower()}:80/");
            resultingYaml
                .Should()
                .HaveKey("proxy.http")
                .And.HaveValue($"http://{proxyHost.ToLower()}:80/");
        }

        [Theory]
        [InlineAutoData]
        public void Should_Correctly_Replace_When_Proxy_Values_Specified(
            Mock<ISession> sessionMock,
            string proxyScheme,
            string proxyHost,
            int proxyPort,
            string proxyUser,
            string proxyPassword)
        {
            var datadogYaml = @"
# proxy:
#   https: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTPS>:<PORT>
#   http: http://<USERNAME>:<PASSWORD>@<PROXY_SERVER_FOR_HTTP>:<PORT>
#   no_proxy:
#     - <HOSTNAME-1>
#     - <HOSTNAME-2>
";
            sessionMock.Setup(session => session["PROXY_HOST"]).Returns($"{proxyScheme}://{proxyHost}");
            sessionMock.Setup(session => session["PROXY_PORT"]).Returns(proxyPort.ToString);
            sessionMock.Setup(session => session["PROXY_USER"]).Returns(proxyUser);
            sessionMock.Setup(session => session["PROXY_PASSWORD"]).Returns(proxyPassword);
            var r = ConfigCustomActions.ReplaceProperties(datadogYaml, sessionMock.Object);
            var resultingYaml = r.ToYaml();
            var expectedUri =
                $"{proxyScheme.ToLower()}://{proxyUser}:{proxyPassword}@{proxyHost.ToLower()}:{proxyPort}/";
            resultingYaml
                .Should()
                .HaveKey("proxy.https")
                .And.HaveValue(expectedUri);
            resultingYaml
                .Should()
                .HaveKey("proxy.http")
                .And.HaveValue(expectedUri);
        }
    }
}
