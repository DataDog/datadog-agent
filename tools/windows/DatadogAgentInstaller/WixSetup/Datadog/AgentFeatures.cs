using System.Collections.Generic;
using WixSharp;

namespace WixSetup.Datadog
{
    public class AgentFeatures
    {
        public const string MainApplicationName = "Datadog Agent";
        public const string NpmFeatureName = "Network Performance Monitoring";

        public Feature MainApplication { get; }

        public Feature Npm { get; }

        public AgentFeatures()
        {
            Npm = new Feature(
                NpmFeatureName,
                description: string.Empty,
                configurableDir: "PROJECTLOCATION")
            {
                Id = new Id("NPM"),
                Attributes = new Dictionary<string, string>
                {
                    {"AllowAdvertise", "no"},
                    {"Absent", "allow"},
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
                Attributes = new Dictionary<string, string>
                {
                    {"AllowAdvertise", "no"},
                    {"Absent", "disallow"},
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
