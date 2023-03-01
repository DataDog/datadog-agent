using WixSharp;

namespace WixSetup
{
    public static class Conditions
    {
        public static readonly Condition LicenseAccepted = Condition.Create("LicenseAccepted = \"1\"");
        public static readonly Condition DatadogYamlExists = Condition.Create("DATADOGYAMLEXISTS");
        public static readonly Condition DatadogYamlDoesntExists = Condition.NOT(DatadogYamlExists);
        public static readonly Condition FirstInstall = Condition.Create("(NOT Installed AND NOT WIX_UPGRADE_DETECTED)");
        public static readonly Condition Upgrading = Condition.Create("(WIX_UPGRADE_DETECTED AND NOT (REMOVE=\"ALL\"))");
        public static readonly Condition Uninstalling = Condition.Create("(Installed AND (REMOVE=\"ALL\") AND NOT (WIX_UPGRADE_DETECTED OR UPGRADINGPRODUCTCODE))");
        public static readonly Condition Maintenance = Condition.Create("(Installed AND NOT Upgrading AND NOT Uninstalling AND NOT UPGRADINGPRODUCTCODE)");
        public static readonly Condition RemovingForUpgrade = Condition.Create("((REMOVE=\"ALL\") AND UPGRADINGPRODUCTCODE)");
    }
}
