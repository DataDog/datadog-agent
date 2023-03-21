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
            Npm = new Feature(NpmFeatureName, description: string.Empty, isEnabled: false, allowChange: true, configurableDir: "PROJECTLOCATION")
            {
                Id = new Id("NPM")
            };
            MainApplication = new Feature(MainApplicationName, description: string.Empty, isEnabled: true, allowChange: false, configurableDir: "PROJECTLOCATION")
            {
                Id = new Id("MainApplication")
            };
            MainApplication.Children.Add(Npm);
        }
    }
}
