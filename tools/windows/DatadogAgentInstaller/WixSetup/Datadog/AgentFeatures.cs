using WixSharp;

namespace WixSetup.Datadog
{
    public class AgentFeatures
    {
        public Feature MainApplication { get; }

        public Feature Npm { get; }

        public AgentFeatures()
        {
            Npm = new Feature("NPM", description: "Network Performance Monitoring", isEnabled: false, allowChange: true, configurableDir: "PROJECTLOCATION");
            MainApplication = new Feature("MainApplication", description: "Datadog Agent", isEnabled: true, allowChange: false, configurableDir: "PROJECTLOCATION");
            MainApplication.Children.Add(Npm);
        }
    }
}
