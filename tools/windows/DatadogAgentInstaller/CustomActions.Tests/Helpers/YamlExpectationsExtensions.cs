using YamlDotNet.RepresentationModel;

namespace CustomActions.Tests.Helpers
{
    /// <summary>
    /// Helper class to make some tests more legible.
    /// It mimics Ruby Spec language.
    /// </summary>
    public static class YamlExpectationsExtensions
    {
        public static void HasApmEnabled(this YamlStream yaml)
        {
            yaml
                .Should()
                .HaveKey("apm_config.enabled")
                .And.HaveValue("true");
        }

        public static void HasLogsEnabled(this YamlStream yaml)
        {
            yaml
                .Should()
                .HaveKey("logs_config");
            yaml
                .Should()
                .HaveKey("logs_enabled")
                .And.HaveValue("true");
        }

        public static void HasProcessEnabled(this YamlStream yaml)
        {
            yaml
                .Should()
                .HaveKey("process_config.process_collection.enabled")
                .And.HaveValue("true");
        }
    }
}
