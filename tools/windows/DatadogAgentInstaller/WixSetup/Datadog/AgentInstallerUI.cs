using System.Drawing;
using System.Xml.Linq;
using WixSharp;
using WixSharp.Controls;

namespace WixSetup.Datadog
{
    // ReSharper disable once InconsistentNaming
    public class AgentInstallerUI
    {
        // ReSharper disable once InconsistentNaming
        public CustomUI CustomUI { get; }

        public AgentInstallerUI(IWixProjectEvents wixProjectEvents, AgentCustomActions agentCustomActions)
        {
            wixProjectEvents.WixSourceGenerated += OnWixSourceGenerated;

            CustomUI = new CustomUI();
            // NOTE: CustomActions called from dialog Controls will not be able to add messages to the log.
            //       If possible, prefer adding the custom action to an install sequence.
            //       https://learn.microsoft.com/en-us/windows/win32/msi/doaction-controlevent

            CustomUI.On(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.LicenseAgreementDlg, Condition.NOT_Installed));
            CustomUI.On(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg, Conditions.Installed_AND_PATCH));

            CustomUI.On(NativeDialogs.LicenseAgreementDlg, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));
            CustomUI.On(NativeDialogs.LicenseAgreementDlg, Buttons.Next, new ShowDialog(NativeDialogs.CustomizeDlg, Conditions.LicenseAccepted));

            CustomUI.On(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg, Condition.Installed) { Order = 1 });
            CustomUI.On(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(NativeDialogs.LicenseAgreementDlg, Condition.NOT_Installed) { Order = 2 });
            CustomUI.On(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.ApiKeyDialog) { Order = 1 });

            CustomUI.On(Dialogs.ApiKeyDialog, Buttons.Next, new ShowDialog(Dialogs.SiteSelectionDialog));
            CustomUI.On(Dialogs.ApiKeyDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg, Condition.NOT_Installed));
            CustomUI.On(Dialogs.ApiKeyDialog, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg, Conditions.Installed_AND_NOT_PATCH));

            CustomUI.On(Dialogs.SiteSelectionDialog, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog));
            CustomUI.On(Dialogs.SiteSelectionDialog, Buttons.Back, new ShowDialog(Dialogs.ApiKeyDialog));

            CustomUI.On(Dialogs.AgentUserDialog, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            CustomUI.On(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(Dialogs.SiteSelectionDialog));

            CustomUI.On(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(Dialogs.AgentUserDialog, Condition.NOT_Installed | Condition.Create("WixUI_InstallMode = \"Change\"")) { Order = 1 });
            CustomUI.On(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg, Conditions.Installed_AND_NOT_PATCH) { Order = 2 });
            CustomUI.On(NativeDialogs.VerifyReadyDlg, Buttons.Next, new ShowDialog(NativeDialogs.WelcomeDlg, Conditions.Installed_AND_PATCH) { Order = 3 });

            CustomUI.On(NativeDialogs.MaintenanceWelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.MaintenanceTypeDlg));

            CustomUI.On(NativeDialogs.MaintenanceTypeDlg, "ChangeButton", new ShowDialog(NativeDialogs.CustomizeDlg));
            CustomUI.On(NativeDialogs.MaintenanceTypeDlg, Buttons.Repair, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            CustomUI.On(NativeDialogs.MaintenanceTypeDlg, Buttons.Remove, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            CustomUI.On(NativeDialogs.MaintenanceTypeDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceWelcomeDlg));

            CustomUI.On(NativeDialogs.ExitDialog, Buttons.Finish, new CloseDialog { Order = 9999 });
        }

        public void OnWixSourceGenerated(XDocument document)
        {
            var ui = document.Root.Select("Product/UI");
            // Need to customize here since color is not supported with standard methods
            ui.AddTextStyle("WixUI_Font_Normal_White", new Font("Tahoma", 8), Color.White);
            ui.AddTextStyle("WixUI_Font_Bigger_White", new Font("Tahoma", 12), Color.White);
            ui.AddTextStyle("WixUI_Font_Title_White", new Font("Tahoma", 9, FontStyle.Bold), Color.White);
        }
    }
}
