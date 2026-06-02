using System.Security.Principal;
using Datadog.CustomActions.Native;
using FluentAssertions;
using Moq;
using Xunit;

namespace CustomActions.Tests.ConfigureUserCustomActions
{
    /// <summary>
    /// Unit tests for the DDAGENTUSER_KEEP_RIGHTS opt-out behavior.
    /// </summary>
    public class ConfigureUserAccountRightsTests
    {
        public ConfigureUserCustomActionsTestSetup Test { get; } = new();

        [Theory]
        [InlineData("1")]
        [InlineData("true")]
        [InlineData("TRUE")]
        [InlineData("yes")]
        [InlineData("Yes")]
        public void ShouldKeepUserAccountRights_Returns_True_For_Truthy_Values(string value)
        {
            Test.Session
                .Setup(session => session[Datadog.CustomActions.ConfigureUserCustomActions.KeepRightsPropertyName])
                .Returns(value);

            Test.Create().ShouldKeepUserAccountRights().Should().BeTrue();
        }

        [Theory]
        [InlineData("")]
        [InlineData(null)]
        [InlineData("0")]
        [InlineData("false")]
        [InlineData("no")]
        [InlineData("anything-else")]
        public void ShouldKeepUserAccountRights_Returns_False_For_Other_Values(string value)
        {
            Test.Session
                .Setup(session => session[Datadog.CustomActions.ConfigureUserCustomActions.KeepRightsPropertyName])
                .Returns(value);

            Test.Create().ShouldKeepUserAccountRights().Should().BeFalse();
        }

        [Fact]
        public void ConfigureUserAccountRights_Skips_SeDeny_Rights_But_Still_Grants_SeServiceLogonRight_When_Flag_Set()
        {
            Test.Session
                .Setup(session => session[Datadog.CustomActions.ConfigureUserCustomActions.KeepRightsPropertyName])
                .Returns("1");

            Test.Create().ConfigureUserAccountRights();

            // SeServiceLogonRight must always be granted: the Agent service depends on it.
            Test.NativeMethods.Verify(
                n => n.AddPrivilege(It.IsAny<SecurityIdentifier>(), AccountRightsConstants.SeServiceLogonRight),
                Times.Once);

            // The SeDeny* rights must NOT be re-applied when the operator opts out.
            Test.NativeMethods.Verify(
                n => n.AddPrivilege(It.IsAny<SecurityIdentifier>(), AccountRightsConstants.SeDenyInteractiveLogonRight),
                Times.Never);
            Test.NativeMethods.Verify(
                n => n.AddPrivilege(It.IsAny<SecurityIdentifier>(), AccountRightsConstants.SeDenyNetworkLogonRight),
                Times.Never);
            Test.NativeMethods.Verify(
                n => n.AddPrivilege(It.IsAny<SecurityIdentifier>(), AccountRightsConstants.SeDenyRemoteInteractiveLogonRight),
                Times.Never);
        }

        [Fact]
        public void ConfigureUserAccountRights_Adds_All_Rights_When_Flag_Not_Set()
        {
            Test.Create().ConfigureUserAccountRights();

            Test.NativeMethods.Verify(
                n => n.AddPrivilege(It.IsAny<SecurityIdentifier>(), AccountRightsConstants.SeDenyInteractiveLogonRight),
                Times.Once);
            Test.NativeMethods.Verify(
                n => n.AddPrivilege(It.IsAny<SecurityIdentifier>(), AccountRightsConstants.SeDenyNetworkLogonRight),
                Times.Once);
            Test.NativeMethods.Verify(
                n => n.AddPrivilege(It.IsAny<SecurityIdentifier>(), AccountRightsConstants.SeDenyRemoteInteractiveLogonRight),
                Times.Once);
            Test.NativeMethods.Verify(
                n => n.AddPrivilege(It.IsAny<SecurityIdentifier>(), AccountRightsConstants.SeServiceLogonRight),
                Times.Once);
        }
    }
}
