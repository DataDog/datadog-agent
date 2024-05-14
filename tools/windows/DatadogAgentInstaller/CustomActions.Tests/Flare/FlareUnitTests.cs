using System.Collections.Generic;
using System.IO;
using AutoFixture.Xunit2;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using FluentAssertions;
using Moq;
using Xunit;

namespace CustomActions.Tests.Flare
{
    public class FlareUnitTests
    {
        [Theory]
        [InlineAutoData("aaaa", "", "test@datadoghq.com")]
        [InlineAutoData("aaaa", "eeee", "iiiii")]
        public void Flare_Should_Post_Empty_Logs_If_Log_File_Doesnt_Exist(
            string apiKey,
            string site,
            string email,
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

            // Malformed log
            sessionMock.Setup(session => session["MsiLogFileLocation"]).Returns("ZX:\\install.log");

            var sut = new Datadog.CustomActions.Flare(httpClientMock.Object, sessionMock.Object);
            sut.Send(email);

            var payload = new System.Collections.Specialized.NameValueCollection
            {
                { "os", "Windows" },
                { "version", CiInfo.PackageVersion },
                { "log", string.Empty },
                { "email", email },
                { "apikey", apiKey },
                { "variant", "windows_installer" }
            };
            httpClientMock.Verify(c => c.Post(
                $"https://api.{site}/agent_stats/report_failure",
                payload,
                new Dictionary<string, string>()
            ));
            sessionMock.Verify(s => s.Log(
                "Log file \"ZX:\\install.log\" does not exist",
                It.IsAny<string>(),
                It.IsAny<string>(),
                It.IsAny<int>()));
        }

        [Theory]
        [InlineAutoData("aaaa", "", "test@datadoghq.com")]
        [InlineAutoData("aaaa", "eeee", "iiiii")]
        public void Flare_Should_Post_Empty_Logs_If_MsiLogFileLocation_Empty(
            string apiKey,
            string site,
            string email,
            Mock<IInstallerHttpClient> httpClientMock,
            Mock<ISession> sessionMock)
        {
            if (site == string.Empty)
            {
                site = "datadoghq.com";
            }

            sessionMock.Setup(session => session["SITE"]).Returns(site);
            sessionMock.Setup(session => session["APIKEY"]).Returns(apiKey);

            var sut = new Datadog.CustomActions.Flare(httpClientMock.Object, sessionMock.Object);
            sut.Send(email);
            var payload = new System.Collections.Specialized.NameValueCollection
            {
                { "os", "Windows" },
                { "version", CiInfo.PackageVersion },
                { "log", string.Empty },
                { "email", email },
                { "apikey", apiKey },
                { "variant", "windows_installer" }
            };
            httpClientMock.Verify(c => c.Post(
                $"https://api.{site}/agent_stats/report_failure",
                payload,
                new Dictionary<string, string>()
            ));
            sessionMock.Verify(s => s.Log(
                "Property MsiLogFileLocation is empty",
                It.IsAny<string>(),
                It.IsAny<string>(),
                It.IsAny<int>()));
        }

        [Theory]
        [InlineAutoData("aaaa", "", "test@datadoghq.com")]
        [InlineAutoData("aaaa", "eeee", "iiiii")]
        public void Flare_Should_Post_Logs(
            string apiKey,
            string site,
            string email,
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

            // Use this unit test file as the "logs"
            var logLocation = new System.Diagnostics.StackTrace(true).GetFrame(0).GetFileName();
            logLocation.Should().NotBeNullOrEmpty();
            sessionMock.Setup(session => session["MsiLogFileLocation"]).Returns(logLocation);
            // ReSharper disable once AssignNullToNotNullAttribute
            var log = File.ReadAllText(logLocation);

            var sut = new Datadog.CustomActions.Flare(httpClientMock.Object, sessionMock.Object);
            sut.Send(email);

            var payload = new System.Collections.Specialized.NameValueCollection
            {
                { "os", "Windows" },
                { "version", CiInfo.PackageVersion },
                { "log", log },
                { "email", email },
                { "apikey", apiKey },
                { "variant", "windows_installer" }
            };
            httpClientMock.Verify(c => c.Post(
                $"https://api.{site}/agent_stats/report_failure",
                payload,
                new Dictionary<string, string>()
            ));
        }
    }
}
