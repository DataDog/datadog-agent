using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Moq;
using System;

namespace CustomActions.Tests.Service
{
    public class ServiceCustomActionsTestSetup : SessionTestBaseSetup
    {
        public Mock<INativeMethods> NativeMethods { get; } = new();
        public Mock<IServiceController> ServiceController { get; } = new();
        public Mock<IRegistryServices> RegistryServices { get; } = new();
        public Mock<IDirectoryServices> DirectoryServices { get; } = new();
        public Mock<IFileServices> FileServices { get; } = new();
        public Mock<IFileSystemServices> FileSystemServices { get; } = new();

        public ServiceCustomActionsTestSetup()
        {
            // No services by default
            ServiceController.SetupGet(s => s.Services).Returns(Array.Empty<IWindowsService>());
        }

        public ServiceCustomAction Create()
        {
            return new ServiceCustomAction(
                Session.Object,
                rollbackDataName: null,
                nativeMethods: NativeMethods.Object,
                registryServices: RegistryServices.Object,
                directoryServices: DirectoryServices.Object,
                fileServices: FileServices.Object,
                serviceController: ServiceController.Object,
                fileSystemServices: FileSystemServices.Object);
        }
    }
}


