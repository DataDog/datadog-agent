using Datadog.CustomActions;
using FluentAssertions;
using WixToolset.Dtf.WindowsInstaller;
using Xunit;

namespace CustomActions.Tests.Prerequisites
{
    public class EnsureSecureConfigRootTests : SessionTestBaseSetup
    {
        // Uninstalling an Agent whose install state (the registry values that record the configuration
        // directory) was removed or corrupted leaves APPLICATIONDATADIRECTORY unset. The configuration
        // directory owner check must not fail in that case, otherwise the custom action returns Failure
        // and the MSI aborts with exit code 1603, making a broken install impossible to uninstall.
        // See incident 58787.
        [Fact]
        public void EnsureSecureConfigRoot_Succeeds_When_ConfigRoot_Is_Not_Set()
        {
            PrerequisitesCustomActions.EnsureSecureConfigRoot(Session.Object)
                .Should()
                .Be(ActionResult.Success);
        }

        [Fact]
        public void EnsureSecureConfigRootUI_Reports_Valid_When_ConfigRoot_Is_Not_Set()
        {
            PrerequisitesCustomActions.EnsureSecureConfigRoot(Session.Object, calledFromUIControl: true)
                .Should()
                .Be(ActionResult.Success);

            Properties.Should()
                .Contain(PrerequisitesCustomActions.ConfigRootValidProperty, "True");
        }
    }
}
