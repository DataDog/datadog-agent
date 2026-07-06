using System.Collections.Generic;
using WixSharp;

namespace WixSetup.Datadog_Agent
{
    public class AgentFeatures
    {
        public const string MainApplicationName = "Datadog Agent";
        public const string NpmFeatureName = "Network Performance Monitoring";
        public const string AiUsageNativeHostFeatureName = "AI Usage Native Host";

        public Feature MainApplication { get; }

        public Feature Npm { get; }

        public Feature AiUsageNativeHost { get; }

        public AgentFeatures()
        {
            // Starting with 7.45, there is no restriction on closed source and NPM has been included in
            // the MainApplication feature. However, docs and release management tools use ADDLOCAL=NPM;
            // if there is no such feature then the install errors out. So we keep an empty feature around
            // to maintain backwards comparability.
            Npm = new Feature(
                NpmFeatureName,
                description: string.Empty,
                configurableDir: "PROJECTLOCATION")
            {
                Id = new Id("NPM"),
                // WiX 5 migration: Absent was renamed to AllowAbsent, values changed to yes/no
                Attributes = new Dictionary<string, string>
                {
                    {"AllowAdvertise", "no"},
                    {"AllowAbsent", "yes"},
                    {"Display", "hidden"},
                    {"InstallDefault", "local"},
                    {"TypicalDefault", "install"},
                    {"Level", "100"}
                }
            };

            // AI-usage Chrome native messaging host artifacts (binary, HKLM NativeMessagingHosts
            // registrations, and the yaml.example) install ONLY when End-User Device Monitoring
            // (EUDM) is enabled. The feature is absent by default (Level 100 > INSTALLLEVEL 1); the
            // FeatureCondition lowers the level to 1 (installed) when DD_INFRASTRUCTURE_MODE is
            // "end_user_device". DD_INFRASTRUCTURE_MODE is re-hydrated from the registry by the
            // ReadInstallState CA before CostFinalize, so this decision is stable across
            // upgrades/repairs that don't re-supply the property on the command line.
            AiUsageNativeHost = new Feature(
                AiUsageNativeHostFeatureName,
                description: string.Empty,
                configurableDir: "PROJECTLOCATION")
            {
                Id = new Id("AiUsageNativeHost"),
                // WiX 5 migration: Absent was renamed to AllowAbsent, values changed to yes/no
                Attributes = new Dictionary<string, string>
                {
                    {"AllowAdvertise", "no"},
                    {"AllowAbsent", "yes"},
                    {"Display", "hidden"},
                    {"InstallDefault", "local"},
                    {"TypicalDefault", "install"},
                    {"Level", "100"}
                },
                Condition = new FeatureCondition("DD_INFRASTRUCTURE_MODE=\"end_user_device\"", level: 1)
            };

            MainApplication = new Feature(
                MainApplicationName,
                description: string.Empty,
                configurableDir: "PROJECTLOCATION")
            {
                Id = new Id("MainApplication"),
                // WiX 5 migration: Absent was renamed to AllowAbsent, values changed to yes/no
                Attributes = new Dictionary<string, string>
                {
                    {"AllowAdvertise", "no"},
                    {"AllowAbsent", "no"},
                    {"Display", "collapse"},
                    {"InstallDefault", "local"},
                    {"TypicalDefault", "install"},
                },
                Children = new List<Feature>
                {
                    Npm,
                    AiUsageNativeHost
                }
            };
        }
    }
}
