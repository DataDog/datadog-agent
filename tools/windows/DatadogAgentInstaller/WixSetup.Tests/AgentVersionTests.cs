using FluentAssertions;
using Xunit;

namespace WixSetup.Tests
{
    public class AgentVersionTests
    {
        [Fact]
        public void Should_Version_Be_7_99_0_0_By_Default()
        {
            // Explicit null to avoid env var influence.
            var version = new Datadog.AgentVersion(null);
            version.PackageVersion.Should().Be("7.99.0");
            version.Version.Major.Should().Be(7);
            version.Version.Minor.Should().Be(99);
            version.Version.Build.Should().Be(0);
            version.Version.Revision.Should().Be(0);
        }

        [Fact]
        public void Should_Parse_Stable_Version_OmnibusFormat_Correctly()
        {
            var packageVersion = "6.35.0";
            var version = new Datadog.AgentVersion(packageVersion);
            version.PackageVersion.Should().Be(packageVersion);
            version.Version.Major.Should().Be(6);
            version.Version.Minor.Should().Be(35);
            version.Version.Build.Should().Be(0);
            version.Version.Revision.Should().Be(0);
        }

        [Fact]
        public void Should_Parse_Nightly_Version_OmnibusFormat_Correctly()
        {
            // Output of inv agent.version --omnibus-format on a nightly
            var packageVersion = "7.40.0~rc.2+git.309.1240df2";
            var version = new Datadog.AgentVersion(packageVersion);
            version.PackageVersion.Should().Be(packageVersion);
            version.Version.Major.Should().Be(7);
            version.Version.Minor.Should().Be(40);
            version.Version.Build.Should().Be(0);
            version.Version.Revision.Should().Be(2);
        }

        [Fact]
        public void Should_Parse_Nightly_Version_UrlSafe_Correctly()
        {
            // Output of inv agent.version --url-safe on an RC
            var packageVersion = "7.43.1-rc.3.git.485.14b9337";
            var version = new Datadog.AgentVersion(packageVersion);
            version.PackageVersion.Should().Be(packageVersion);
            version.Version.Major.Should().Be(7);
            version.Version.Minor.Should().Be(43);
            version.Version.Build.Should().Be(1);
            version.Version.Revision.Should().Be(3);
        }
    }
}
