using System;
using System.IO;
using AutoFixture.Xunit2;
using CustomActions.Tests;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using FluentAssertions;
using Microsoft.Deployment.WindowsInstaller;
using Moq;
using Xunit;

namespace CustomActions.Tests.InstallSource
{
    public class UpdateInstallSourceCustomActionTests : SessionTestBaseSetup
    {

        [Fact]
        public void UpdateInstallSource_WithFleetInstall_ShouldSkipAndReturnSuccess()
        {
            // Arrange
            Session.Setup(s => s["FLEET_INSTALL"]).Returns("1");
            Session.Setup(s => s["DATABASE"]).Returns("C:\\test.msi");
            Session.Setup(s => s["PROJECTLOCATION"]).Returns("C:\\Program Files\\Datadog");

            // Act - Call the actual custom action
            var result = UpdateInstallSourceCustomAction.UpdateInstallSource(Session.Object);

            // Assert
            result.Should().Be(ActionResult.Success);
            // Verify that the fleet install property was checked
            Session.Verify(s => s["FLEET_INSTALL"], Times.Once);
            Session.Verify(s => s.RunCommand(It.IsAny<string>(), It.IsAny<string>()), Times.Never);
        }

        [Fact]
        public void UpdateInstallSource_WithFipsInstall_ShouldSkipAndReturnSuccess()
        {
            // Arrange
            Session.Setup(s => s["FLEET_INSTALL"]).Returns("0");
            Session.Setup(s => s["AgentFlavor"]).Returns(Constants.FipsFlavor);
            Session.Setup(s => s["DATABASE"]).Returns("C:\\test.msi");
            Session.Setup(s => s["PROJECTLOCATION"]).Returns("C:\\Program Files\\Datadog");

            // Act - Call the actual custom action
            var result = UpdateInstallSourceCustomAction.UpdateInstallSource(Session.Object);

            // Assert
            result.Should().Be(ActionResult.Success);
            // Verify that both properties were checked
            Session.Verify(s => s["FLEET_INSTALL"], Times.Once);
            Session.Verify(s => s["AgentFlavor"], Times.Once);
            Session.Verify(s => s.RunCommand(It.IsAny<string>(), It.IsAny<string>()), Times.Never);
        }

    }
}
