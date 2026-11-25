using System.IO;
using System.Security.Principal;
using CustomActions.Tests.Helpers;
using CustomActions.Tests.ProcessUserCustomActions;
using Datadog.CustomActions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Moq;
using WixToolset.Dtf.WindowsInstaller;
using FluentAssertions;

namespace CustomActions.Tests.ConfigureUserCustomActions
{
    public class UninstallUserTests
    {
        public ConfigureUserCustomActionsTestSetup Test { get; } = new();

        [ElevatedFact]
        public void UninstallUser_Removes_Stored_Account_Access_When_Account_Is_Not_Set()
        {
            var configRoot = Path.Combine(Path.GetTempPath(), "datadog-uninstall-user-test");
            Test.Session.Setup(session => session["APPLICATIONDATADIRECTORY"]).Returns(configRoot);

            var storedKey = new Mock<IRegistryKey>();
            storedKey.Setup(k => k.GetValue("installedDomain")).Returns("HOST");
            storedKey.Setup(k => k.GetValue("installedUser")).Returns("ddagentuser");
            Test.RegistryServices
                .Setup(r => r.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey))
                .Returns(storedKey.Object);

            var userSid = new SecurityIdentifier("S-1-5-21-1-2-3-1001");
            Test.NativeMethods.Setup(n => n.LookupAccountName(
                    @"HOST\ddagentuser",
                    out It.Ref<string>.IsAny,
                    out It.Ref<string>.IsAny,
                    out It.Ref<SecurityIdentifier>.IsAny,
                    out It.Ref<SID_NAME_USE>.IsAny))
                .Callback(new LookupAccountNameDelegate(
                    (
                        string _,
                        out string user,
                        out string domain,
                        out SecurityIdentifier sid,
                        out SID_NAME_USE nameUse
                    ) =>
                    {
                        user = "ddagentuser";
                        domain = "HOST";
                        sid = userSid;
                        nameUse = SID_NAME_USE.SidTypeUser;
                    }))
                .Returns(true);

            Test.Create("UninstallUser").UninstallUser().Should().Be(ActionResult.Success);

            // The per-file removal loop only runs for a resolved account.
            Test.FileSystemServices.Verify(f => f.Exists(Path.Combine(configRoot, "conf.d")));
        }
    }
}
