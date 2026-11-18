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
    public class ServiceStartupTests : ServiceCustomActionsTestSetup
    {
        private void SetServices(params string[] names)
        {
            ServiceController.SetupGet(c => c.Services).Returns(names.Select(name =>
            {
                var service = new Mock<IWindowsService>();
                service.SetupGet(s => s.ServiceName).Returns(name);
                service.SetupGet(s => s.Status).Returns(ServiceControllerStatus.Stopped);
                return service.Object;
            }).ToArray());
        }

        [Theory]
        [InlineData("")]
        [InlineData("{incoming-product}")]
        public void RemovalDisablesEvenStoppedServicesBeforeStopping(string upgradingProductCode)
        {
            SetServices(ServiceCustomAction.ServicesToDisable);
            Mock.Get(ServiceController.Object.Services.First()).SetupGet(s => s.Status)
                .Returns(ServiceControllerStatus.Running);
            Session.Object["REMOVE"] = "ALL";
            Session.Object["UPGRADINGPRODUCTCODE"] = upgradingProductCode;
            var events = new List<string>();
            ServiceController.Setup(c => c.SetStartType(It.IsAny<string>(), ServiceStartMode.Disabled))
                .Callback<string, ServiceStartMode>((name, _) => events.Add(name));
            ServiceController.Setup(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()))
                .Callback(() => events.Add("stopped"));

            Assert.Equal(ActionResult.Success, Create().StopDDServicesForInstall());

            Assert.Equal(ServiceCustomAction.ServicesToDisable.Concat(new[] { "stopped" }), events);
        }

        [Theory]
        [InlineData("", "")]
        [InlineData("", "{outgoing-product}")]
        [InlineData("SomeFeature", "")]
        public void InstallUpgradeAndRepairStopWithoutDisabling(string remove, string upgradeDetected)
        {
            SetServices(Constants.AgentServiceName);
            Mock.Get(ServiceController.Object.Services.Single()).SetupGet(s => s.Status)
                .Returns(ServiceControllerStatus.Running);
            Session.Object["REMOVE"] = remove;
            Session.Object["WIX_UPGRADE_DETECTED"] = upgradeDetected;

            Assert.Equal(ActionResult.Success, Create().StopDDServicesForInstall());

            ServiceController.Verify(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()), Times.Never);
            ServiceController.Verify(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()), Times.Once);
        }

        [Fact]
        public void RemovalDoesNotDisableServicesWhenRollbackIsDisabled()
        {
            SetServices(ServiceCustomAction.ServicesToDisable);
            Session.Object["REMOVE"] = "ALL";
            Session.Object["RollbackDisabled"] = "1";

            Assert.Equal(ActionResult.Failure, Create().StopDDServicesForInstall());

            ServiceController.Verify(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()), Times.Never);
        }

        [Fact]
        public void PartialDisableFailureCanBeUndoneWithPackageDefaults()
        {
            SetServices(ServiceCustomAction.ServicesToDisable);
            Session.Object["REMOVE"] = "ALL";
            ServiceController.Setup(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Disabled))
                .Throws(new InvalidOperationException("access denied"));

            Assert.Equal(ActionResult.Failure, Create().StopDDServicesForInstall());
            Assert.Equal(ActionResult.Success, ServiceCustomAction.EnableDDServices(Session.Object, ServiceController.Object, continueOnError: true));

            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void EnableAppliesPackageDefaultsWithAutomaticBeforeDelayedAndCoreAgentLast()
        {
            SetServices(ServiceCustomAction.ActiveServices.Concat(new[] { "retired-service" }).ToArray());
            var events = new List<string>();
            ServiceController.Setup(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()))
                .Callback<string, ServiceStartMode>((name, _) => events.Add(name));
            ServiceController.Setup(c => c.SetDelayedAutoStart(Constants.AgentServiceName, true))
                .Callback(() => events.Add("delayed"));

            Assert.Equal(ActionResult.Success, ServiceCustomAction.EnableDDServices(Session.Object, ServiceController.Object));

            Assert.Equal(ServiceCustomAction.ActiveServices.Concat(new[] { "delayed" }), events);
            Assert.Equal(Constants.AgentServiceName, ServiceCustomAction.ActiveServices.Last());
            foreach (var name in ServiceCustomAction.ActiveServices.Where(n => n != Constants.AgentServiceName))
            {
                ServiceController.Verify(c => c.SetStartType(name, ServiceStartMode.Manual), Times.Once);
            }
            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.SetStartType("retired-service", It.IsAny<ServiceStartMode>()), Times.Never);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Theory]
        [InlineData("1")]
        [InlineData("true")]
        [InlineData("TRUE")]
        public void InstallOnlyEnablesServicesWithoutStartingThem(string installOnly)
        {
            SetServices(ServiceCustomAction.ActiveServices);
            Session.Object["DD_INSTALL_ONLY"] = installOnly;

            Assert.Equal(ActionResult.Success, Create().StartDDServices());

            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.SetDelayedAutoStart(Constants.AgentServiceName, true), Times.Once);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void StartActionEnablesAllServicesBeforeStartingCoreAgent()
        {
            SetServices(ServiceCustomAction.ActiveServices);
            var events = new List<string>();
            ServiceController.Setup(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()))
                .Callback<string, ServiceStartMode>((name, _) => events.Add(name));
            ServiceController.Setup(c => c.StartService(Constants.AgentServiceName, It.IsAny<TimeSpan>()))
                .Callback(() => events.Add("started"));

            Assert.Equal(ActionResult.Success, Create().StartDDServices());

            Assert.Equal(ServiceCustomAction.ActiveServices.Concat(new[] { "started" }), events);
        }

        [Theory]
        [InlineData("")]
        [InlineData("1")]
        public void StartActionFailsWithoutStartingCoreAgentWhenEnableFails(string installOnly)
        {
            SetServices(ServiceCustomAction.ActiveServices);
            Session.Object["DD_INSTALL_ONLY"] = installOnly;
            ServiceController.Setup(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Manual))
                .Throws(new System.ComponentModel.Win32Exception(5));

            Assert.Equal(ActionResult.Failure, Create().StartDDServices());

            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, It.IsAny<ServiceStartMode>()), Times.Never);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void RemovalRollbackContinuesEnablingOtherServicesAfterFailure()
        {
            SetServices(ServiceCustomAction.ActiveServices);
            ServiceController.Setup(c => c.SetStartType(Constants.ProcmgrServiceName, ServiceStartMode.Manual))
                .Throws(new InvalidOperationException("access denied"));

            Assert.Equal(ActionResult.Failure, ServiceCustomAction.EnableDDServices(Session.Object, ServiceController.Object, continueOnError: true));

            ServiceController.Verify(c => c.SetStartType(Constants.SystemProbeServiceName, ServiceStartMode.Manual), Times.Once);
            ServiceController.Verify(c => c.SetStartType(Constants.AgentServiceName, ServiceStartMode.Automatic), Times.Once);
            ServiceController.Verify(c => c.StartService(It.IsAny<string>(), It.IsAny<TimeSpan>()), Times.Never);
        }

        [Fact]
        public void MissingOptionalServicesAreSkipped()
        {
            SetServices(Constants.AgentServiceName);
            Session.Object["REMOVE"] = "ALL";

            Assert.Equal(ActionResult.Success, Create().StopDDServicesForInstall());
            Assert.Equal(ActionResult.Success, ServiceCustomAction.EnableDDServices(Session.Object, ServiceController.Object));

            ServiceController.Verify(c => c.SetStartType(It.IsNotIn(Constants.AgentServiceName), It.IsAny<ServiceStartMode>()), Times.Never);
        }

        [Fact]
        public void IncomingRollbackStopsWithoutDisablingServicesOwnedByReleasedMSIs()
        {
            SetServices(Constants.AgentServiceName);
            Mock.Get(ServiceController.Object.Services.Single()).SetupGet(s => s.Status)
                .Returns(ServiceControllerStatus.Running);

            Assert.Equal(ActionResult.Success, Create().StartDDServicesRollback());

            ServiceController.Verify(c => c.StopService(Constants.AgentServiceName, It.IsAny<TimeSpan>()), Times.Once);
            ServiceController.Verify(c => c.SetStartType(It.IsAny<string>(), It.IsAny<ServiceStartMode>()), Times.Never);
        }
    }
}
