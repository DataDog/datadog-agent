using System;
using System.Collections.Generic;
using System.Linq;
using System.ServiceProcess;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Rollback;
using Moq;
using Newtonsoft.Json;
using Xunit;
using WixToolset.Dtf.WindowsInstaller;

namespace CustomActions.Tests.Service
{
    public class ServiceStartupTests : ServiceCustomActionsTestSetup
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
        public void SerializedRollbackDataRestoresStartupSettingsWithoutStartingServices()
        {
            SetServices(Constants.AgentServiceName);
            var data = new ServiceStartupRollbackData(new List<ServiceCustomAction.StartupSettings>
            {
                new ServiceCustomAction.StartupSettings
                {
                    Name = Constants.AgentServiceName,
                    StartType = ServiceStartMode.Automatic,
                    DelayedAutoStart = true
                }
            });

            var restored = JsonConvert.DeserializeObject<ServiceStartupRollbackData>(JsonConvert.SerializeObject(data));
            restored.Restore(Session.Object, FileSystemServices.Object, ServiceController.Object);

            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.SetDelayedAutoStart(Constants.AgentServiceName, true), Times.Once);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void PersistsSettingsAndDisablesEvenStoppedServicesBeforeStopping()
        {
            SetServices(ServiceCustomAction.ProtectedServices);
            var events = new List<string>();
            List<ServiceCustomAction.StartupSettings> saved = null;
            ServiceController.Setup(c => c.SetStartType(It.IsAny<string>(), ServiceStartMode.Disabled))
                .Callback<string, ServiceStartMode>((name, _) => events.Add(name));

            ServiceCustomAction.DisableServices(ServiceController.Object, settings =>
            {
                saved = settings;
                events.Add("saved");
            }, () => events.Add("stop"));

            Assert.Equal(new[] { "saved", Constants.AgentServiceName, Constants.ProcmgrServiceName,
                Constants.InstallerServiceName, "stop" }, events);
            Assert.Equal(ServiceStartMode.Automatic, saved[0].StartType);
            Assert.True(saved[0].DelayedAutoStart);
        }

        [Theory]
        [InlineData(false)]
        [InlineData(true)]
        public void StopActionOnlySnapshotsAndDisablesForTheOuterTransaction(bool nestedRemoval)
        {
            SetServices(Constants.AgentServiceName);
            var service = Mock.Get(ServiceController.Object.Services.Single());
            service.SetupGet(s => s.Status).Returns(ServiceControllerStatus.Running);
            if (nestedRemoval)
            {
                Session.Object["UPGRADINGPRODUCTCODE"] = "{outer-product-code}";
                // A future old MSI sees the temporary Disabled value left by the outer MSI.
                service.SetupGet(s => s.StartType).Returns(ServiceStartMode.Disabled);
            }
            var events = new List<string>();
            ServiceController.Setup(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Disabled))
                .Callback(() => events.Add("disabled"));
            ServiceController.Setup(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()))
                .Callback(() => events.Add("stopped"));

            var result = Create().StopDDServicesWithSnapshot(settings =>
            {
                Assert.Equal(ServiceStartMode.Automatic, Assert.Single(settings).StartType);
                events.Add("saved");
            });

            Assert.Equal(ActionResult.Success, result);
            Assert.Equal(nestedRemoval ? new[] { "stopped" } : new[] { "saved", "disabled", "stopped" }, events);
        }

        [Theory]
        [InlineData(false)]
        [InlineData(true)]
        public void StopActionDoesNotMutateServicesWithoutRollbackProtection(bool rollbackDisabled)
        {
            SetServices(ServiceCustomAction.ProtectedServices);
            Session.Object["RollbackDisabled"] = rollbackDisabled ? "1" : "";
            var saveAttempted = false;

            var result = Create().StopDDServicesWithSnapshot(_ =>
            {
                saveAttempted = true;
                throw new InvalidOperationException("Cannot persist snapshot");
            });

            Assert.Equal(ActionResult.Failure, result);
            Assert.Equal(!rollbackDisabled, saveAttempted);
            ServiceController.Verify(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()), Times.Never);
            ServiceController.Verify(c => c.StopService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void EnableRestoresPackageDefaultsWithCoreAgentLastWithoutStartingServices()
        {
            SetServices(ServiceCustomAction.ProtectedServices);
            var enabled = new List<string>();
            ServiceController.Setup(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()))
                .Callback<string, ServiceStartMode>((name, _) => enabled.Add(name));

            Assert.Equal(ActionResult.Success, ServiceCustomAction.EnableDDServices(Session.Object, ServiceController.Object));
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
            Assert.Equal(ActionResult.Success, ServiceCustomAction.EnableDDServices(Session.Object, ServiceController.Object));
            ServiceController.Verify(c => c.SetStartType(Constants.ProcmgrServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
        }

        [Fact]
        public void EnableFailureFailsTheActionWithoutEnablingCoreAgent()
        {
            SetServices(ServiceCustomAction.ProtectedServices);
            ServiceController.Setup(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Manual))
                .Throws(new System.ComponentModel.Win32Exception(5));
            Assert.Equal(ActionResult.Failure, ServiceCustomAction.EnableDDServices(Session.Object, ServiceController.Object));
            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
        }

        [Fact]
        public void MissingServicesAreNotDisabled()
        {
            SetServices(Constants.AgentServiceName);
            ServiceCustomAction.DisableServices(ServiceController.Object,
                settings => Assert.Single(settings), () => { });
            ServiceController.Verify(c => c.SetStartType(Constants.ProcmgrServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
            ServiceController.Verify(c => c.SetStartType(Constants.InstallerServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
        }

        [Fact]
        public void SnapshotFailureDoesNotChangeOrStopServices()
        {
            SetServices(ServiceCustomAction.ProtectedServices);
            var stopped = false;
            Assert.Throws<InvalidOperationException>(() => ServiceCustomAction.DisableServices(ServiceController.Object,
                _ => throw new InvalidOperationException("disk full"), () => stopped = true));
            ServiceController.Verify(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()), Times.Never);
            Assert.False(stopped);
        }

        [Fact]
        public void PartialDisableFailureLeavesOriginalSettingsAvailableForRollback()
        {
            SetServices(ServiceCustomAction.ProtectedServices);
            List<ServiceCustomAction.StartupSettings> saved = null;
            var stopped = false;
            ServiceController.Setup(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Disabled))
                .Throws(new InvalidOperationException("access denied"));
            Assert.Throws<InvalidOperationException>(() => ServiceCustomAction.DisableServices(ServiceController.Object,
                settings => saved = settings, () => stopped = true));
            Assert.False(stopped);

            ServiceCustomAction.RestoreServiceStartup(Session.Object, ServiceController.Object, saved);
            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.SetDelayedAutoStart(Constants.AgentServiceName, true), Times.Once);
            ServiceController.Verify(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Manual), Times.Once);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void StopFailureCanRestoreSettingsWithoutRestarting()
        {
            SetServices(ServiceCustomAction.ProtectedServices);
            List<ServiceCustomAction.StartupSettings> saved = null;
            Assert.Throws<TimeoutException>(() => ServiceCustomAction.DisableServices(ServiceController.Object,
                settings => saved = settings, () => throw new TimeoutException("service did not stop")));
            Assert.True(ServiceCustomAction.RestoreServiceStartup(Session.Object, ServiceController.Object, saved));
            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void RollbackPreservesOriginallyDisabledServiceAndContinuesAfterRestoreFailure()
        {
            SetServices(ServiceCustomAction.ProtectedServices);
            ServiceController.Setup(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic))
                .Throws(new InvalidOperationException("access denied"));
            ServiceCustomAction.RestoreServiceStartup(Session.Object, ServiceController.Object, new[]
            {
                new ServiceCustomAction.StartupSettings { Name = Constants.InstallerServiceName, StartType = ServiceStartMode.Disabled },
                new ServiceCustomAction.StartupSettings { Name = "missing", StartType = ServiceStartMode.Manual },
                new ServiceCustomAction.StartupSettings { Name = Constants.AgentServiceName, StartType = ServiceStartMode.Automatic }
            });
            ServiceController.Verify(c => c.SetStartType(Constants.InstallerServiceName, ServiceStartMode.Disabled), Times.Once);
            ServiceController.Verify(c => c.SetStartType("missing", It.IsAny<ServiceStartMode>()), Times.Never);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }
    }
}
