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
            Npm = new Feature(NpmFeatureName, description: NpmFeatureName, isEnabled: false, allowChange: true, configurableDir: "PROJECTLOCATION");
            MainApplication = new Feature(MainApplicationName, description: MainApplicationName, isEnabled: true, allowChange: false, configurableDir: "PROJECTLOCATION");
            MainApplication.Children.Add(Npm);
        }
    }
}
