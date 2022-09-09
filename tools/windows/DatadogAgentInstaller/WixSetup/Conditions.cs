using WixSharp;

namespace WixSetup
{
    public static class Conditions
    {
        public static readonly Condition LicenseAccepted = Condition.Create("LicenseAccepted = \"1\"");
        public static readonly Condition Installed_AND_NOT_PATCH = Condition.Installed & Condition.NOT(Condition.Create("PATCH"));
        public static readonly Condition Installed_AND_PATCH = Condition.Installed & Condition.Create("PATCH");
    }
}
