using System.Collections.Generic;
using WixSharp;
using WixSharp.Controls;

namespace WixSetup.Datadog_Agent
{
    // ReSharper disable once InconsistentNaming
    public class AgentInstallerUI : DatadogCustomUI
    {
        public AgentInstallerUI(IWixProjectEvents wixProjectEvents, AgentCustomActions agentCustomActions)
        : base(wixProjectEvents)
        {
            // ARPNOMODIFY=1 disables the "Change" button in the Control Panel, so remove it so that we have
            // our button.
            // https://learn.microsoft.com/en-us/windows/win32/msi/arpnomodify
            Properties.Remove("ARPNOMODIFY");
            DialogRefs = new List<string>
            {
                CommonDialogs.BrowseDlg,
                CommonDialogs.DiskCostDlg,
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
                .AddXmlInclude("dialogs/ddlicense.wxi")
                .AddXmlInclude("dialogs/errormodaldlg.wxi")
                // Replaces CommonDialogs.ErrorDlg, which is not referenced above
                .AddXmlInclude("dialogs/errordlg.wxi");

            // NOTE: CustomActions called from dialog Controls will not be able to add messages to the log.
            //       If possible, prefer adding the custom action to an install sequence.
            //       https://learn.microsoft.com/en-us/windows/win32/msi/doaction-controlevent

            // Refuse to go past the first dialog when the configuration directory cannot be trusted,
            // see PrerequisitesCustomActions.EnsureSecureConfigRoot. The same check runs in the
            // InstallExecuteSequence, which is what a silent install relies on.
            // Set by the EnsureSecureConfigRootUI custom action
            var configRootValid = new Condition("DDConfigRoot_Valid = \"True\"");
            var configRootInvalid = new Condition("DDConfigRoot_Valid <> \"True\"");

            // Fresh install track
            OnFreshInstall(NativeDialogs.WelcomeDlg, Buttons.Next,
                new ExecuteCustomAction(agentCustomActions.EnsureSecureConfigRootUI) { Order = 1 },
                new SpawnDialog(Dialogs.ErrorModalDialog, configRootInvalid) { Order = 2 },
                new ShowDialog(Dialogs.LicenseAgreementDlg, configRootValid) { Order = 3 });
            OnFreshInstall(Dialogs.LicenseAgreementDlg, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));
            OnFreshInstall(Dialogs.LicenseAgreementDlg, Buttons.Next, new ShowDialog(NativeDialogs.CustomizeDlg, Conditions.LicenseAccepted));
            OnFreshInstall(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(Dialogs.LicenseAgreementDlg));
            OnFreshInstall(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.ApiKeyDialog, Conditions.NOT_DatadogYamlExists));
            OnFreshInstall(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog, Conditions.DatadogYamlExists));
            OnFreshInstall(Dialogs.ApiKeyDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnFreshInstall(Dialogs.ApiKeyDialog, Buttons.Next, new ShowDialog(Dialogs.SiteSelectionDialog));
            OnFreshInstall(Dialogs.SiteSelectionDialog, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog));
            OnFreshInstall(Dialogs.SiteSelectionDialog, Buttons.Back, new ShowDialog(Dialogs.ApiKeyDialog));
            OnFreshInstall(Dialogs.AgentUserDialog, Buttons.Next,
                new ExecuteCustomAction(agentCustomActions.ProcessDdAgentUserCredentialsUI) { Order = 1 },
                new SpawnDialog(Dialogs.ErrorModalDialog, new Condition("DDAgentUser_Valid <> \"True\"")) { Order = 2 },
                new ShowDialog(NativeDialogs.VerifyReadyDlg, new Condition("DDAgentUser_Valid = \"True\"")) { Order = 3 }
            );
            OnFreshInstall(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(Dialogs.SiteSelectionDialog, Conditions.NOT_DatadogYamlExists));
            OnFreshInstall(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg, Conditions.DatadogYamlExists));
            OnFreshInstall(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(Dialogs.AgentUserDialog));

            // Upgrade track
            OnUpgrade(NativeDialogs.WelcomeDlg, Buttons.Next,
                new ExecuteCustomAction(agentCustomActions.EnsureSecureConfigRootUI) { Order = 1 },
                new SpawnDialog(Dialogs.ErrorModalDialog, configRootInvalid) { Order = 2 },
                new ShowDialog(NativeDialogs.CustomizeDlg, configRootValid) { Order = 3 });
            OnUpgrade(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(NativeDialogs.WelcomeDlg));
            OnUpgrade(NativeDialogs.CustomizeDlg, Buttons.Next, new ShowDialog(Dialogs.AgentUserDialog));
            OnUpgrade(Dialogs.AgentUserDialog, Buttons.Back, new ShowDialog(NativeDialogs.CustomizeDlg));
            OnUpgrade(Dialogs.AgentUserDialog, Buttons.Next,
                new ExecuteCustomAction(agentCustomActions.ProcessDdAgentUserCredentialsUI) { Order = 1 },
                new SpawnDialog(Dialogs.ErrorModalDialog, new Condition("DDAgentUser_Valid <> \"True\"")) { Order = 2 },
                new ShowDialog(NativeDialogs.VerifyReadyDlg, new Condition("DDAgentUser_Valid = \"True\"")) { Order = 3 });
            OnUpgrade(NativeDialogs.VerifyReadyDlg, Buttons.Back, new ShowDialog(Dialogs.AgentUserDialog));

            // Maintenance track
            //
            // Checked once here, on the way out of the Welcome dialog, the same as the fresh-install
            // and upgrade tracks above: Change, Repair, and Remove all require a trusted
            // configuration directory (see PrerequisitesCustomActions.EnsureSecureConfigRoot), so one
            // check here covers all three buttons below.
            OnMaintenance(NativeDialogs.MaintenanceWelcomeDlg, Buttons.Next,
                new ExecuteCustomAction(agentCustomActions.EnsureSecureConfigRootUI) { Order = 1 },
                new SpawnDialog(Dialogs.ErrorModalDialog, configRootInvalid) { Order = 2 },
                new ShowDialog(NativeDialogs.MaintenanceTypeDlg, configRootValid) { Order = 3 });
            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceWelcomeDlg));

            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, "ChangeButton",
                new SetProperty("PREVIOUS_PAGE", NativeDialogs.MaintenanceTypeDlg) { Order = 1 },
                new ShowDialog(NativeDialogs.CustomizeDlg) { Order = 2 });

            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, Buttons.Repair,
                new SetProperty("PREVIOUS_PAGE", NativeDialogs.MaintenanceTypeDlg) { Order = 1 },
                new ShowDialog(Dialogs.AgentUserDialog) { Order = 2 });

            OnMaintenance(NativeDialogs.MaintenanceTypeDlg, Buttons.Remove,
                new ShowDialog(NativeDialogs.VerifyReadyDlg));

            OnMaintenance(NativeDialogs.CustomizeDlg, Buttons.Back, new ShowDialog(NativeDialogs.MaintenanceTypeDlg));
            OnMaintenance(NativeDialogs.CustomizeDlg, Buttons.Next,
                new SetProperty("PREVIOUS_PAGE", NativeDialogs.CustomizeDlg),
                new ShowDialog(Dialogs.AgentUserDialog));

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
    }
}
