using System.Collections.Generic;
using WixSharp;
using WixSharp.Controls;

namespace WixSetup.Datadog_Installer
{
    public class DatadogInstallerUI : DatadogCustomUI
    {
        public DatadogInstallerUI(IWixProjectEvents wixProjectEvents, DatadogInstallerCustomActions customActions)
        : base(wixProjectEvents)
        {
            DialogRefs = new List<string>
            {
                CommonDialogs.ErrorDlg,
            };

            this.AddXmlInclude("dialogs/apikeydlg.wxi")
                .AddXmlInclude("dialogs/sitedlg.wxi")
                .AddXmlInclude("dialogs/fatalError.wxi");

            OnFreshInstall(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(Dialogs.ApiKeyDialog));
            OnFreshInstall(Dialogs.ApiKeyDialog, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));
            OnFreshInstall(Dialogs.ApiKeyDialog, Buttons.Next, new ShowDialog(Dialogs.SiteSelectionDialog));
            OnFreshInstall(Dialogs.SiteSelectionDialog, Buttons.Back, new ShowDialog(Dialogs.ApiKeyDialog));
            OnFreshInstall(Dialogs.SiteSelectionDialog, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg));

            OnUpgrade(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            OnUpgrade(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));

            OnMaintenance(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            OnMaintenance(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));

            On(NativeDialogs.ExitDialog, Buttons.Finish, new CloseDialog { Order = 9999 });
            On(Dialogs.FatalErrorDialog, "OpenMsiLog", new ExecuteCustomAction(customActions.OpenMsiLog));
        }
    }
}
