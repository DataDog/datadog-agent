using System.Collections.Generic;
using System.Drawing;
using System.Xml.Linq;
using WixSharp;
using WixSharp.Controls;

namespace WixSetup.Datadog
{
    // ReSharper disable once InconsistentNaming
    public class AgentInstallerUI : CustomUI
    {
        private CustomUI OnFreshInstall(string dialog, string control, params DialogAction[] handlers)
        {
            handlers.ForEach(h => h.Condition &= Conditions.FirstInstall);
            return On(dialog, control, handlers);
        }

        private CustomUI OnUpgrade(string dialog, string control, params DialogAction[] handlers)
        {
            handlers.ForEach(h => h.Condition &= Conditions.Upgrading);
            return On(dialog, control, handlers);
        }

        private CustomUI OnMaintenance(string dialog, string control, params DialogAction[] handlers)
        {
            handlers.ForEach(h => h.Condition &= Conditions.Maintenance);
            return On(dialog, control, handlers);
        }

        public AgentInstallerUI(IWixProjectEvents wixProjectEvents, AgentCustomActions agentCustomActions)
        {
            wixProjectEvents.WixSourceGenerated += OnWixSourceGenerated;
            DialogRefs = new List<string>
            {
                CommonDialogs.BrowseDlg,
                CommonDialogs.DiskCostDlg,
                CommonDialogs.ErrorDlg,
                CommonDialogs.FilesInUse,
                CommonDialogs.MsiRMFilesInUse,
                CommonDialogs.PrepareDlg,
                CommonDialogs.ProgressDlg,
                CommonDialogs.ResumeDlg,
                CommonDialogs.UserExit
            };

            this.AddXmlInclude("dialogs/apikeydlg.wxi")
                .AddXmlInclude("dialogs/sitedlg.wxi")
                .AddXmlInclude("dialogs/fatalError.wxi")
                .AddXmlInclude("dialogs/sendFlaredlg.wxi")
                .AddXmlInclude("dialogs/ddagentuserdlg.wxi");

            // NOTE: CustomActions called from dialog Controls will not be able to add messages to the log.
            //       If possible, prefer adding the custom action to an install sequence.
            //       https://learn.microsoft.com/en-us/windows/win32/msi/doaction-controlevent

            // Fresh install track
            OnFreshInstall(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.LicenseAgreementDlg));
            OnFreshInstall(NativeDialogs.LicenseAgreementDlg, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));
            OnFreshInstall(NativeDialogs.LicenseAgreementDlg, Buttons.Next, new ShowDialog(NativeDialogs.CustomizeDlg, Conditions.LicenseAccepted));
            OnFreshInstall(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(NativeDialogs.LicenseAgreementDlg));
            OnFreshInstall(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.ApiKeyDialog, Conditions.NOT_DatadogYamlExists));
            OnFreshInstall(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog, Conditions.DatadogYamlExists));
            OnFreshInstall(Dialogs.ApiKeyDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnFreshInstall(Dialogs.ApiKeyDialog, Buttons.Next, new ShowDialog(Dialogs.SiteSelectionDialog));
            OnFreshInstall(Dialogs.SiteSelectionDialog, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog));
            OnFreshInstall(Dialogs.SiteSelectionDialog, Buttons.Back, new ShowDialog(Dialogs.ApiKeyDialog));
            OnFreshInstall(Dialogs.AgentUserDialog, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            OnFreshInstall(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(Dialogs.SiteSelectionDialog, Conditions.NOT_DatadogYamlExists));
            OnFreshInstall(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg, Conditions.DatadogYamlExists));
            OnFreshInstall(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(Dialogs.AgentUserDialog));
            // Upgrade track
            OnUpgrade(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnUpgrade(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));
            OnUpgrade(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog));
            OnUpgrade(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnUpgrade(Dialogs.AgentUserDialog, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            OnUpgrade(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(Dialogs.AgentUserDialog));
            // Maintenance track
            OnMaintenance(NativeDialogs.MaintenanceWelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.MaintenanceTypeDlg));
            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceWelcomeDlg));
            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, "ChangeButton", new ShowDialog(NativeDialogs.CustomizeDlg));
            OnMaintenance(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg));
            OnMaintenance(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, Buttons.Repair, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, Buttons.Remove, new ShowDialog(NativeDialogs.VerifyReadyDlg));
            // There's not way to know if we were on MaintenanceTypeDlg or CustomizeDlg previously, so go back the furthest.
            OnMaintenance(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg));

            On(NativeDialogs.ExitDialog, Buttons.Finish, new CloseDialog { Order = 9999 });

            On(Dialogs.FatalErrorDialog, "OpenMsiLog", new ExecuteCustomAction(agentCustomActions.OpenMsiLog));
            On(Dialogs.FatalErrorDialog, "SendFlare", new ShowDialog(Dialogs.SendFlareDialog));

            On(Dialogs.SendFlareDialog, Buttons.Back, new ShowDialog(Dialogs.FatalErrorDialog));
            On(Dialogs.SendFlareDialog, "SendFlare", new ExecuteCustomAction(agentCustomActions.SendFlare));
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
