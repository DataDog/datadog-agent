using Datadog.CustomActions.Interfaces;
using Moq;

namespace CustomActions.Tests.ConfigureUserCustomActions
{
    public class ConfigureUserCustomActionsTestSetup : SessionTestBaseSetup
    {
        public Mock<INativeMethods> NativeMethods { get; } = new();
        public Mock<IRegistryServices> RegistryServices { get; } = new();
        public Mock<IFileSystemServices> FileSystemServices { get; } = new();
        public Mock<IServiceController> ServiceController { get; } = new();

        public Datadog.CustomActions.ConfigureUserCustomActions Create(string rollbackDataName = "ConfigureUser")
        {
            return new Datadog.CustomActions.ConfigureUserCustomActions(
                Session.Object,
                rollbackDataName,
                NativeMethods.Object,
                RegistryServices.Object,
                FileSystemServices.Object,
                ServiceController.Object
            );
        }
    }
}
