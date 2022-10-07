using YamlDotNet.RepresentationModel;

namespace CustomActions.Tests
{
    public static class YamlAssertionsExtensions
    {
        public static YamlAssertions Should(this YamlStream instance)
        {
            return new YamlAssertions(instance.Documents[0].RootNode);
        }
    }
}
