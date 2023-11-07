using System;
using System.Collections.Generic;
using AutoFixture.Xunit2;
using Moq;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Xunit;
using ISession = Datadog.CustomActions.Interfaces.ISession;

namespace CustomActions.Tests.Telemetry
{
    public class TelemetryUnitTests
    {
        [Theory]
        [InlineAutoData("aaaa", "", "agent.installation.success", "be7b577b-00d9-50a4-aa8d-345df57fd6f5", "Aliens")]
        [InlineAutoData("aaaa", "datadoghq.eu", "agent.installation.failure", "f56820a0-170e-57a3-b4ce-82ac5032bf31", "Aliens")]
        public void ReportTelemetry_Should_Post_Telemetry(
            string apiKey,
            string site,
            string eventName,
            string installId,
            string origin,
            Mock<IInstallerHttpClient> httpClientMock,
            Mock<ISession> sessionMock
        )
        {
            if (site == string.Empty)
            {
                site = "datadoghq.com";
            }
            sessionMock.Setup(session => session["SITE"]).Returns(site);
            sessionMock.Setup(session => session["APIKEY"]).Returns(apiKey);

            System.Environment.SetEnvironmentVariable("DD_INSTALL_ID", installId);
            System.Environment.SetEnvironmentVariable("DD_ORIGIN", origin);
            var sut = new Datadog.CustomActions.Telemetry(httpClientMock.Object, sessionMock.Object);
            sut.ReportTelemetry(eventName);
            httpClientMock.Verify(c => c.Post(
                $"https://instrumentation-telemetry-intake.{site}/api/v2/apmtelemetry", @$"
{{
    ""request_type"": ""apm-onboarding-event"",
    ""api_version"": ""v1"",
    ""payload"": {{
        ""event_name"": ""{eventName}"",
        ""tags"": {{
            ""agent_platform"": ""windows"",
            ""agent_version"": ""{CiInfo.PackageVersion}"",
            ""script_version"": ""{typeof(CiInfo).Assembly.GetName().Version}"",
            ""install_id"": ""{installId}"",
            ""origin"": ""{origin}""
        }}
    }}
}}"
                ,
                new Dictionary<string, string>
                {
                    { "DD-Api-Key", apiKey },
                    { "Content-Type", "application/json" },
                }
            ));
        }

        [Theory]
        [InlineAutoData("", "")]
        [InlineAutoData("", "datadoghq.eu")]
        public void ReportTelemetry_Should_Not_Post_Telemetry(
            string apiKey,
            string site,
            string eventName,
            Mock<IInstallerHttpClient> httpClientMock,
            Mock<ISession> sessionMock
        )
        {
            if (site == string.Empty)
            {
                site = "datadoghq.com";
            }
            sessionMock.Setup(session => session["SITE"]).Returns(site);
            sessionMock.Setup(session => session["APIKEY"]).Returns(apiKey);

            var sut = new Datadog.CustomActions.Telemetry(httpClientMock.Object, sessionMock.Object);
            sut.ReportTelemetry(eventName);
            httpClientMock.VerifyNoOtherCalls();
        }
    }
}
