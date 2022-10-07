using System.IO;
using YamlDotNet.RepresentationModel;

namespace CustomActions.Tests
{
    public static class YamlExtensions
    {
        public static YamlStream ToYaml(this string value)
        {
            var resultYaml = new YamlStream();
            resultYaml.Load(new StringReader(value));
            return resultYaml;
        }
    }
}
