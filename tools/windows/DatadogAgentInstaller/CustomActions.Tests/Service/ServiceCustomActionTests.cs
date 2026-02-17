using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using FluentAssertions;
using WixToolset.Dtf.WindowsInstaller;
using Moq;
using System;
using System.ComponentModel;
using System.ServiceProcess;
using Xunit;

namespace CustomActions.Tests.Service
{
    public class ServiceCustomActionTests : ServiceCustomActionsTestSetup
    {
        public ServiceCustomActionsTestSetup Test { get; } = new();

        [Fact]
        public void ReturnsTrue_ForWin32Exception1060()
        {
            var ex = new Win32Exception(1060); // ERROR_SERVICE_DOES_NOT_EXIST
            Assert.True(ServiceCustomAction.IsServiceDoesNotExistError(ex));
        }

        [Fact]
        public void ReturnsTrue_ForWrappedWin32Exception1060()
        {
            var inner = new Win32Exception(1060);
            var ex = new Exception("wrapper", inner);
            Assert.True(ServiceCustomAction.IsServiceDoesNotExistError(ex));
        }

        [Fact]
        public void ReturnsTrue_ForInvalidOperationMessage()
        {
            var ex = new InvalidOperationException("The specified service does not exist as an installed service");
            Assert.True(ServiceCustomAction.IsServiceDoesNotExistError(ex));
        }

        [Fact]
        public void ReturnsFalse_ForOtherWin32Exception()
        {
            var ex = new Win32Exception(5); // Access denied
            Assert.False(ServiceCustomAction.IsServiceDoesNotExistError(ex));
        }

        [Fact]
        public void ReturnsFalse_ForUnrelatedException()
        {
            var ex = new InvalidOperationException("Some other message");
            Assert.False(ServiceCustomAction.IsServiceDoesNotExistError(ex));
        }

        [Fact]
        public void ExceptionFilter_Catches1060AndSkipsFallback()
        {
            var handledByFilter = false;
            var handledByFallback = false;

            try
            {
                throw new InvalidOperationException("error message", new Win32Exception(1060));
            }
            catch (Exception ex) when (ServiceCustomAction.IsServiceDoesNotExistError(ex))
            {
                handledByFilter = true;
            }
            catch (Exception)
            {
                handledByFallback = true;
            }

            Assert.True(handledByFilter);
            Assert.False(handledByFallback);
        }

        [Fact]
        public void ExceptionFilter_SkipsForOtherExceptions()
        {
            var handledByFilter = false;
            var handledByFallback = false;

            try
            {
                throw new Win32Exception(5);
            }
            catch (Exception ex) when (ServiceCustomAction.IsServiceDoesNotExistError(ex))
            {
                handledByFilter = true;
            }
            catch (Exception)
            {
                handledByFallback = true;
            }

            Assert.False(handledByFilter);
            Assert.True(handledByFallback);
        }

        /// <summary>
        /// Tests that a late "service does not exist" error is ignored 
        /// </summary>
        /// <remarks>
        /// We had an instance where the Agent service somehow got returned from Enum but then raised a "not found" error
        /// when trying to send the stop signal, see https://datadoghq.atlassian.net/browse/WINA-1776
        /// </remarks>
        [Fact]
        public void StopDDServices_Treats1060AsSuccess()
        {
            var serviceMock = new Mock<IWindowsService>();
            serviceMock.SetupGet(s => s.ServiceName).Returns(Constants.AgentServiceName);
            serviceMock.SetupGet(s => s.DisplayName).Returns("Datadog Agent");
            serviceMock.SetupGet(s => s.Status).Returns(ServiceControllerStatus.Running);
            serviceMock.Setup(s => s.Refresh());

            Test.ServiceController.SetupGet(c => c.Services).Returns(new[] { serviceMock.Object });
            Test.ServiceController
                .Setup(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()))
                .Throws(new InvalidOperationException("error", new Win32Exception(1060)));

            Test.Create()
                .StopDDServices(false)
                .Should()
                .Be(ActionResult.Success);

            Test.ServiceController.Verify(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()), Times.Once);
        }

        [Fact]
        public void StopDDServices_ContinuesOnNon1060WhenContinueOnErrorTrue()
        {
            var serviceMock = new Mock<IWindowsService>();
            serviceMock.SetupGet(s => s.ServiceName).Returns(Constants.AgentServiceName);
            serviceMock.SetupGet(s => s.DisplayName).Returns("Datadog Agent");
            serviceMock.SetupGet(s => s.Status).Returns(ServiceControllerStatus.Running);
            serviceMock.Setup(s => s.Refresh());

            Test.ServiceController.SetupGet(c => c.Services).Returns(new[] { serviceMock.Object });
            Test.ServiceController
                .Setup(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()))
                .Throws(new Exception("boom"));

            Test.Create()
                .StopDDServices(true)
                .Should()
                .Be(ActionResult.Success);

            Test.ServiceController.Verify(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()), Times.Once);
        }

        [Fact]
        public void StopDDServices_FailsOnNon1060WhenContinueOnErrorFalse()
        {
            var serviceMock = new Mock<IWindowsService>();
            serviceMock.SetupGet(s => s.ServiceName).Returns(Constants.AgentServiceName);
            serviceMock.SetupGet(s => s.DisplayName).Returns("Datadog Agent");
            serviceMock.SetupGet(s => s.Status).Returns(ServiceControllerStatus.Running);
            serviceMock.Setup(s => s.Refresh());

            Test.ServiceController.SetupGet(c => c.Services).Returns(new[] { serviceMock.Object });
            Test.ServiceController
                .Setup(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()))
                .Throws(new Exception("boom"));

            Test.Create()
                .StopDDServices(false)
                .Should()
                .Be(ActionResult.Failure);

            Test.ServiceController.Verify(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()), Times.Once);
        }

        [Fact]
        public void StopDDServices_DoesNotCallStopWhenServiceNotFound()
        {
            Test.ServiceController.SetupGet(c => c.Services).Returns(Array.Empty<IWindowsService>());

            Test.Create()
                .StopDDServices(false)
                .Should()
                .Be(ActionResult.Success);

            Test.ServiceController.Verify(c => c.StopService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }
    }
}


