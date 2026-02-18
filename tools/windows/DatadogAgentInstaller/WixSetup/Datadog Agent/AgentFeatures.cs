using System.Collections.Generic;
using WixSharp;

namespace WixSetup.Datadog_Agent
{
    public class AgentFeatures
    {
        public const string MainApplicationName = "Datadog Agent";
        public const string NpmFeatureName = "Network Performance Monitoring";

        public Feature MainApplication { get; }

        public Feature Npm { get; }

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
                    Npm
                }
            };
        }
    }
}
