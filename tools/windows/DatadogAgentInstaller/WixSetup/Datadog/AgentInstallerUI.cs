using System.Collections.Generic;
using System.Drawing;
using System.Xml.Linq;
using WixSharp;
using WixSharp.Controls;
using WixSharp.Forms;

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
            // ARPNOMODIFY=1 disables the "Change" button in the Control Panel, so remove it so that we have
            // our button.
            // https://learn.microsoft.com/en-us/windows/win32/msi/arpnomodify
            Properties.Remove("ARPNOMODIFY");
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
                .AddXmlInclude("dialogs/ddagentuserdlg.wxi")
                .AddXmlInclude("dialogs/closedsourceconsentdlg.wxi")
                .AddXmlInclude("dialogs/errormodaldlg.wxi");

            // NOTE: CustomActions called from dialog Controls will not be able to add messages to the log.
            //       If possible, prefer adding the custom action to an install sequence.
            //       https://learn.microsoft.com/en-us/windows/win32/msi/doaction-controlevent

            // Fresh install track
            OnFreshInstall(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.LicenseAgreementDlg));
            OnFreshInstall(NativeDialogs.LicenseAgreementDlg, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));
            OnFreshInstall(NativeDialogs.LicenseAgreementDlg, Buttons.Next, new ShowDialog(Dialogs.ClosedSourceConsentDialog, Conditions.LicenseAccepted));
            OnFreshInstall(Dialogs.ClosedSourceConsentDialog, Buttons.Back, new ShowDialog(NativeDialogs.LicenseAgreementDlg));
            OnFreshInstall(Dialogs.ClosedSourceConsentDialog, Buttons.Next, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnFreshInstall(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(Dialogs.ClosedSourceConsentDialog));
            OnFreshInstall(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.ApiKeyDialog, Conditions.NOT_DatadogYamlExists));
            OnFreshInstall(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog, Conditions.DatadogYamlExists));
            OnFreshInstall(Dialogs.ApiKeyDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnFreshInstall(Dialogs.ApiKeyDialog, Buttons.Next, new ShowDialog(Dialogs.SiteSelectionDialog));
            OnFreshInstall(Dialogs.SiteSelectionDialog, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog));
            OnFreshInstall(Dialogs.SiteSelectionDialog, Buttons.Back, new ShowDialog(Dialogs.ApiKeyDialog));
            OnFreshInstall(Dialogs.AgentUserDialog, Buttons.Next,
                new ExecuteCustomAction(agentCustomActions.ProcessDdAgentUserCredentialsUI) { Order = 1},
                new SpawnDialog(Dialogs.ErrorModalDialog, new Condition("DDAgentUser_Valid <> \"True\"")) { Order = 2 },
                new ShowDialog(NativeDialogs.VerifyReadyDlg, new Condition("DDAgentUser_Valid = \"True\"")) { Order = 3 }
            );
            OnFreshInstall(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(Dialogs.SiteSelectionDialog, Conditions.NOT_DatadogYamlExists));
            OnFreshInstall(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg, Conditions.DatadogYamlExists));
            OnFreshInstall(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(Dialogs.AgentUserDialog));

            // Upgrade track
            OnUpgrade(NativeDialogs.WelcomeDlg, Buttons.Next, new ShowDialog(Dialogs.ClosedSourceConsentDialog));
            OnUpgrade(Dialogs.ClosedSourceConsentDialog, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));
            OnUpgrade(Dialogs.ClosedSourceConsentDialog, Buttons.Next, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnUpgrade(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(Dialogs.ClosedSourceConsentDialog));
            OnUpgrade(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog));
            OnUpgrade(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnUpgrade(Dialogs.AgentUserDialog, Buttons.Next,
                new ExecuteCustomAction(agentCustomActions.ProcessDdAgentUserCredentialsUI) { Order = 1 },
                new SpawnDialog(Dialogs.ErrorModalDialog, new Condition("DDAgentUser_Valid <> \"True\"")) { Order = 2 },
                new ShowDialog(NativeDialogs.VerifyReadyDlg, new Condition("DDAgentUser_Valid = \"True\"")) { Order = 3 });
            OnUpgrade(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(Dialogs.AgentUserDialog));

            // Maintenance track
            OnMaintenance(NativeDialogs.MaintenanceWelcomeDlg, Buttons.Next, new ShowDialog(NativeDialogs.MaintenanceTypeDlg));
            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceWelcomeDlg));

            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, "ChangeButton",
                new SetProperty("PREVIOUS_PAGE", NativeDialogs.MaintenanceTypeDlg),
                new ShowDialog(Dialogs.ClosedSourceConsentDialog));

            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, Buttons.Repair,
                new SetProperty("PREVIOUS_PAGE", NativeDialogs.MaintenanceTypeDlg),
                new ShowDialog(Dialogs.AgentUserDialog));

            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, Buttons.Remove,
                new ShowDialog(NativeDialogs.VerifyReadyDlg));

            OnMaintenance(Dialogs.ClosedSourceConsentDialog, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg));
            OnMaintenance(Dialogs.ClosedSourceConsentDialog, Buttons.Next, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnMaintenance(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(Dialogs.ClosedSourceConsentDialog));
            OnMaintenance(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog));

            OnMaintenance(Dialogs.AgentUserDialog, Buttons.Back,
                new ShowDialog(NativeDialogs.CustomizeDlg, new Condition($"PREVIOUS_PAGE = \"{NativeDialogs.CustomizeDlg}\"")) { Order = 1 });
            OnMaintenance(Dialogs.AgentUserDialog, Buttons.Back,
                new ShowDialog(NativeDialogs.MaintenanceTypeDlg, new Condition($"PREVIOUS_PAGE = \"{NativeDialogs.MaintenanceTypeDlg}\"")) { Order = 2 });
            OnMaintenance(Dialogs.AgentUserDialog, Buttons.Next,
                new SetProperty("PREVIOUS_PAGE", Dialogs.AgentUserDialog),
                new ExecuteCustomAction(agentCustomActions.ProcessDdAgentUserCredentialsUI) { Order = 1 },
                new SpawnDialog(Dialogs.ErrorModalDialog, new Condition("DDAgentUser_Valid <> \"True\"")) { Order = 2 },
                new ShowDialog(NativeDialogs.VerifyReadyDlg, new Condition("DDAgentUser_Valid = \"True\"")) { Order = 3 });

            // There's no way to know if we were on MaintenanceTypeDlg or CustomizeDlg previously, so go back the furthest.
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
