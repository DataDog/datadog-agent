using FluentAssertions;
using Xunit;

namespace WixSetup.Tests
{
    public class AgentVersionTests
    {
        [Fact]
        public void Should_Version_Be_7_99_0_2_By_Default()
        {
            // Explicit null to avoid env var influence.
            var version = new Datadog.AgentVersion(null);
            version.PackageVersion.Should().Be("7.99.0");
            version.Version.Major.Should().Be(7);
            version.Version.Minor.Should().Be(99);
            version.Version.Build.Should().Be(0);
            version.Version.Revision.Should().Be(2);
        }

        [Fact]
        public void Should_Parse_Nightly_Version_Correctly()
        {
            // Output of inv -e agent.version --omnibus-format on a nightly
            string packageVersion = "7.40.0~rc.2+git.309.1240df2";
            var version = new Datadog.AgentVersion(packageVersion);
            version.PackageVersion.Should().Be(packageVersion);
            version.Version.Major.Should().Be(7);
            version.Version.Minor.Should().Be(40);
            version.Version.Build.Should().Be(0);
            version.Version.Revision.Should().Be(2);
        }
    }
}
