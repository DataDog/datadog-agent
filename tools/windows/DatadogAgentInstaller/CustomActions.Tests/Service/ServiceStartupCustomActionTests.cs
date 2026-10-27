using System;
using System.Collections.Generic;
using System.Linq;
using System.ServiceProcess;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Moq;
using Xunit;
using WixToolset.Dtf.WindowsInstaller;

namespace CustomActions.Tests.Service
{
    public class ServiceStartupCustomActionTests : ServiceCustomActionsTestSetup
    {
        private void SetServices(params string[] names)
        {
            ServiceController.SetupGet(c => c.Services).Returns(names.Select(name =>
            {
                var service = new Mock<IWindowsService>();
                service.SetupGet(s => s.ServiceName).Returns(name);
                service.SetupGet(s => s.Status).Returns(ServiceControllerStatus.Stopped);
                service.SetupGet(s => s.StartType).Returns(
                    name == Constants.AgentServiceName ? ServiceStartMode.Automatic : ServiceStartMode.Manual);
                service.SetupGet(s => s.DelayedAutoStart).Returns(name == Constants.AgentServiceName);
                return service.Object;
            }).ToArray());
        }

        [Fact]
        public void PersistsSettingsAndDisablesEvenStoppedServicesBeforeStopping()
        {
            SetServices(ServiceStartupCustomAction.ProtectedServices);
            var events = new List<string>();
            List<ServiceStartupCustomAction.StartupSettings> saved = null;
            ServiceController.Setup(c => c.SetStartType(It.IsAny<string>(), ServiceStartMode.Disabled))
                .Callback<string, ServiceStartMode>((name, _) => events.Add(name));

            ServiceStartupCustomAction.Disable(ServiceController.Object, settings =>
            {
                saved = settings;
                events.Add("saved");
            }, () => events.Add("stop"));

            Assert.Equal(new[] { "saved", Constants.AgentServiceName, Constants.ProcmgrServiceName,
                Constants.InstallerServiceName, "stop" }, events);
            Assert.Equal(ServiceStartMode.Automatic, saved[0].StartType);
            Assert.True(saved[0].DelayedAutoStart);
        }

        [Fact]
        public void EnableRestoresPackageDefaultsWithCoreAgentLastWithoutStartingServices()
        {
            SetServices(ServiceStartupCustomAction.ProtectedServices);
            var enabled = new List<string>();
            ServiceController.Setup(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()))
                .Callback<string, ServiceStartMode>((name, _) => enabled.Add(name));

            Assert.Equal(ActionResult.Success, ServiceStartupCustomAction.Enable(Session.Object, ServiceController.Object));
            Assert.Equal(new[] { Constants.InstallerServiceName, Constants.ProcmgrServiceName, Constants.AgentServiceName }, enabled);
            ServiceController.Verify(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Manual), Times.Once);
            ServiceController.Verify(c => c.SetStartType(Constants.InstallerServiceName, ServiceStartMode.Manual), Times.Once);
            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.SetDelayedAutoStart(Constants.AgentServiceName, true), Times.Once);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void EnableSkipsMissingProcmgr()
        {
            SetServices(Constants.AgentServiceName);
            Assert.Equal(ActionResult.Success, ServiceStartupCustomAction.Enable(Session.Object, ServiceController.Object));
            ServiceController.Verify(c => c.SetStartType(Constants.ProcmgrServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
        }

        [Fact]
        public void EnableFailureFailsTheActionWithoutEnablingCoreAgent()
        {
            SetServices(ServiceStartupCustomAction.ProtectedServices);
            ServiceController.Setup(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Manual))
                .Throws(new System.ComponentModel.Win32Exception(5));
            Assert.Equal(ActionResult.Failure, ServiceStartupCustomAction.Enable(Session.Object, ServiceController.Object));
            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
        }

        [Fact]
        public void MissingServicesAreNotDisabled()
        {
            SetServices(Constants.AgentServiceName);
            ServiceStartupCustomAction.Disable(ServiceController.Object,
                settings => Assert.Single(settings), () => { });
            ServiceController.Verify(c => c.SetStartType(Constants.ProcmgrServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
            ServiceController.Verify(c => c.SetStartType(Constants.InstallerServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
        }

        [Fact]
        public void SnapshotFailureDoesNotChangeOrStopServices()
        {
            SetServices(ServiceStartupCustomAction.ProtectedServices);
            var stopped = false;
            Assert.Throws<InvalidOperationException>(() => ServiceStartupCustomAction.Disable(ServiceController.Object,
                _ => throw new InvalidOperationException("disk full"), () => stopped = true));
            ServiceController.Verify(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()), Times.Never);
            Assert.False(stopped);
        }

        [Fact]
        public void PartialDisableFailureLeavesOriginalSettingsAvailableForRollback()
        {
            SetServices(ServiceStartupCustomAction.ProtectedServices);
            List<ServiceStartupCustomAction.StartupSettings> saved = null;
            var stopped = false;
            ServiceController.Setup(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Disabled))
                .Throws(new InvalidOperationException("access denied"));
            Assert.Throws<InvalidOperationException>(() => ServiceStartupCustomAction.Disable(ServiceController.Object,
                settings => saved = settings, () => stopped = true));
            Assert.False(stopped);

            ServiceStartupCustomAction.Restore(Session.Object, ServiceController.Object, saved);
            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.SetDelayedAutoStart(Constants.AgentServiceName, true), Times.Once);
            ServiceController.Verify(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Manual), Times.Once);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void StopFailureCanRestoreSettingsWithoutRestarting()
        {
            SetServices(ServiceStartupCustomAction.ProtectedServices);
            List<ServiceStartupCustomAction.StartupSettings> saved = null;
            Assert.Throws<TimeoutException>(() => ServiceStartupCustomAction.Disable(ServiceController.Object,
                settings => saved = settings, () => throw new TimeoutException("service did not stop")));
            Assert.True(ServiceStartupCustomAction.Restore(Session.Object, ServiceController.Object, saved));
            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void RollbackPreservesOriginallyDisabledServiceAndContinuesAfterRestoreFailure()
        {
            SetServices(ServiceStartupCustomAction.ProtectedServices);
            ServiceController.Setup(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic))
                .Throws(new InvalidOperationException("access denied"));
            ServiceStartupCustomAction.Restore(Session.Object, ServiceController.Object, new[]
            {
                new ServiceStartupCustomAction.StartupSettings { Name = Constants.InstallerServiceName, StartType = ServiceStartMode.Disabled },
                new ServiceStartupCustomAction.StartupSettings { Name = "missing", StartType = ServiceStartMode.Manual },
                new ServiceStartupCustomAction.StartupSettings { Name = Constants.AgentServiceName, StartType = ServiceStartMode.Automatic }
            });
            ServiceController.Verify(c => c.SetStartType(Constants.InstallerServiceName, ServiceStartMode.Disabled), Times.Once);
            ServiceController.Verify(c => c.SetStartType("missing", It.IsAny<ServiceStartMode>()), Times.Never);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }
    }
}
