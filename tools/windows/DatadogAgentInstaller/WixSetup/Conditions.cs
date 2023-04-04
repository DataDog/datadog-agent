using WixSharp;

namespace WixSetup
{
    public static class Conditions
    {
        public static readonly Condition LicenseAccepted = Condition.Create("LicenseAccepted = \"1\"");

        public static readonly Condition DatadogYamlExists = Condition.Create("DATADOGYAMLEXISTS=\"yes\"");

        // ReSharper disable once InconsistentNaming
        public static readonly Condition NOT_DatadogYamlExists = Condition.NOT(DatadogYamlExists);

        public static readonly Condition HasUpgradingProductCode = Condition.Create("UPGRADINGPRODUCTCODE");

        public static readonly Condition WixUpgradeDetected = Condition.Create("WIX_UPGRADE_DETECTED");

        // ReSharper disable once InconsistentNaming
        public static readonly Condition NOT_WixUpgradeDetected = Condition.NOT(WixUpgradeDetected);

        public static readonly Condition FirstInstall = Condition.NOT_Installed & NOT_WixUpgradeDetected;

        public static readonly Condition Upgrading = WixUpgradeDetected & Condition.NOT_BeingRemoved;

        public static readonly Condition Uninstalling = Condition.Installed &
                                                        Condition.BeingUninstalled &
                                                        Condition.NOT(WixUpgradeDetected | HasUpgradingProductCode);

        public static readonly Condition Maintenance = Condition.Installed &
                                                       Condition.NOT(Upgrading) &
                                                       Condition.NOT(Uninstalling) &
                                                       Condition.NOT(HasUpgradingProductCode);

        public static readonly Condition RemovingForUpgrade = Condition.BeingUninstalled &
                                                              HasUpgradingProductCode;
    }
}
