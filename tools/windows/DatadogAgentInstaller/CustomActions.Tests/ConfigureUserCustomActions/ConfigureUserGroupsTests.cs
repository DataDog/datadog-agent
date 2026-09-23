using System;
using System.ComponentModel;
using System.Security.Principal;
using CustomActions.Tests.Helpers;
using FluentAssertions;
using Moq;

namespace CustomActions.Tests.ConfigureUserCustomActions
{
    public class ConfigureUserGroupsTests
    {
        public ConfigureUserCustomActionsTestSetup Test { get; } = new();

        private void VerifyAllGroupWrites(Times times)
        {
            Test.NativeMethods.Verify(
                n => n.AddToGroup(
                    It.IsAny<SecurityIdentifier>(),
                    WellKnownSidType.BuiltinPerformanceMonitoringUsersSid),
                times);
            Test.NativeMethods.Verify(
                n => n.AddToGroup(
                    It.IsAny<SecurityIdentifier>(),
                    WellKnownSidType.BuiltinPerformanceLoggingUsersSid),
                times);
            Test.NativeMethods.Verify(
                n => n.AddToGroup(
                    It.IsAny<SecurityIdentifier>(),
                    It.Is<SecurityIdentifier>(sid => sid.Value == "S-1-5-32-573")),
                times);
        }

        [ElevatedFact]
        public void ReadOnlyDomainControllerSkipsAllGroupWrites()
        {
            Test.NativeMethods.Setup(n => n.IsDomainController()).Returns(true);
            Test.NativeMethods.Setup(n => n.IsReadOnlyDomainController()).Returns(true);

            Test.Create().ConfigureUserGroups();

            VerifyAllGroupWrites(Times.Never());
        }

        [ElevatedFact]
        public void WritableDomainControllerPerformsAllGroupWrites()
        {
            Test.NativeMethods.Setup(n => n.IsDomainController()).Returns(true);
            Test.NativeMethods.Setup(n => n.IsReadOnlyDomainController()).Returns(false);

            Test.Create().ConfigureUserGroups();

            VerifyAllGroupWrites(Times.Once());
        }

        [ElevatedFact]
        public void NonDomainControllerPerformsAllGroupWritesWithoutMetadataLookup()
        {
            Test.NativeMethods.Setup(n => n.IsDomainController()).Returns(false);

            Test.Create().ConfigureUserGroups();

            VerifyAllGroupWrites(Times.Once());
            Test.NativeMethods.Verify(n => n.IsReadOnlyDomainController(), Times.Never());
        }

        [ElevatedFact]
        public void MetadataFailurePropagatesWithoutGroupWrites()
        {
            Test.NativeMethods.Setup(n => n.IsDomainController()).Returns(true);
            Test.NativeMethods.Setup(n => n.IsReadOnlyDomainController()).Throws(new InvalidOperationException("metadata unavailable"));

            Action act = () => Test.Create().ConfigureUserGroups();

            act.Should().Throw<InvalidOperationException>().WithMessage("metadata unavailable");
            VerifyAllGroupWrites(Times.Never());
        }

        [ElevatedFact]
        public void GroupWriteFailurePropagates()
        {
            Test.NativeMethods.Setup(n => n.IsDomainController()).Returns(true);
            Test.NativeMethods.Setup(n => n.IsReadOnlyDomainController()).Returns(false);
            Test.NativeMethods
                .Setup(n => n.AddToGroup(
                    It.IsAny<SecurityIdentifier>(),
                    WellKnownSidType.BuiltinPerformanceMonitoringUsersSid))
                .Throws(new Win32Exception(50));

            Action act = () => Test.Create().ConfigureUserGroups();

            act.Should().Throw<Win32Exception>().Which.NativeErrorCode.Should().Be(50);
        }

        [ElevatedFact]
        public void DomainControllerCheckFailurePropagatesWithoutGroupWrites()
        {
            Test.NativeMethods.Setup(n => n.IsDomainController()).Throws(new InvalidOperationException("role unavailable"));

            Action act = () => Test.Create().ConfigureUserGroups();

            act.Should().Throw<InvalidOperationException>().WithMessage("role unavailable");
            Test.NativeMethods.Verify(n => n.IsReadOnlyDomainController(), Times.Never());
            VerifyAllGroupWrites(Times.Never());
        }
    }
}
