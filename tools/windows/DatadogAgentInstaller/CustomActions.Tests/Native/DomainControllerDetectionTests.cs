using System;
using Datadog.CustomActions.Native;
using FluentAssertions;
using Xunit;

namespace CustomActions.Tests.Native
{
    public class DomainControllerDetectionTests
    {
        private static Win32NativeMethods.DS_DOMAIN_CONTROLLER_INFO_3 Controller(
            string netbiosName,
            string dnsHostName,
            bool isRodc)
        {
            return new Win32NativeMethods.DS_DOMAIN_CONTROLLER_INFO_3
            {
                NetbiosName = netbiosName,
                DnsHostName = dnsHostName,
                fIsRodc = isRodc,
            };
        }

        [Fact]
        public void LocalDnsNameWinsOverRemoteControllerMetadata()
        {
            var entries = new[]
            {
                Controller("REMOTE-DC", "remote-dc.example.com", false),
                Controller("LOCAL-DC", "LOCAL-DC.EXAMPLE.COM", true),
            };

            Win32NativeMethods.IsLocalDomainControllerReadOnly(
                    entries,
                    "local-dc.example.com",
                    "local-dc")
                .Should().BeTrue();
        }

        [Fact]
        public void LocalNetbiosNameWinsRegardlessOfOrderingAndCasing()
        {
            var entries = new[]
            {
                Controller("LOCAL-DC", "local-dc.example.com", false),
                Controller("remote-dc", "remote-dc.example.com", true),
            };

            Win32NativeMethods.IsLocalDomainControllerReadOnly(
                    entries,
                    "unmatched.example.com",
                    "local-dc")
                .Should().BeFalse();
        }

        [Fact]
        public void MissingLocalControllerMetadataReportsAnError()
        {
            var entries = new[]
            {
                Controller("REMOTE-DC", "remote-dc.example.com", true),
            };

            Action act = () => Win32NativeMethods.IsLocalDomainControllerReadOnly(
                entries,
                "local-dc.example.com",
                "local-dc");

            act.Should().Throw<InvalidOperationException>()
                .WithMessage("*did not contain the local controller*");
        }
    }
}
