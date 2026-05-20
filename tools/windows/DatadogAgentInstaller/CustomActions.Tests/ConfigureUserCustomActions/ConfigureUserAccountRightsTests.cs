using System.Security.Principal;
using Datadog.CustomActions.Native;
using FluentAssertions;
using Moq;
using Xunit;

namespace CustomActions.Tests.ConfigureUserCustomActions
{
    /// <summary>
    /// Unit tests for the DDAGENTUSER_KEEP_USER_RIGHTS opt-out behavior
    /// added in FRAGENT-3418.
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
                .Setup(session => session[Datadog.CustomActions.ConfigureUserCustomActions.KeepUserRightsPropertyName])
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
                .Setup(session => session[Datadog.CustomActions.ConfigureUserCustomActions.KeepUserRightsPropertyName])
                .Returns(value);

            Test.Create().ShouldKeepUserAccountRights().Should().BeFalse();
        }

        [Fact]
        public void ConfigureUserAccountRights_Skips_AddPrivilege_And_Logs_When_Flag_Set()
        {
            Test.Session
                .Setup(session => session[Datadog.CustomActions.ConfigureUserCustomActions.KeepUserRightsPropertyName])
                .Returns("1");

            Test.Create().ConfigureUserAccountRights();

            Test.NativeMethods.Verify(
                n => n.AddPrivilege(It.IsAny<SecurityIdentifier>(), It.IsAny<string>()),
                Times.Never);

            // The skip path must warn operators that the agent service still needs SeServiceLogonRight.
            Test.Session.Verify(
                s => s.Log(
                    It.Is<string>(msg => msg.Contains("SeServiceLogonRight")),
                    It.IsAny<string>(),
                    It.IsAny<string>(),
                    It.IsAny<int>()),
                Times.Once);
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
