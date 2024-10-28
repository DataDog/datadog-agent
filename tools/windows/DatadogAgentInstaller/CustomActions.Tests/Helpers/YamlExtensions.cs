using System.IO;
using YamlDotNet.RepresentationModel;

namespace CustomActions.Tests.Helpers
{
    /// <summary>
    /// Extension class to load a <see cref="YamlStream"/> from a <see cref="string"/>.
    /// </summary>
    public static class YamlExtensions
    {
        /// <summary>
        /// Converts a <see cref="string"/> into a <see cref="YamlStream"/>.
        /// </summary>
        /// <param name="value">The string to convert.</param>
        /// <returns>A <see cref="YamlStream"/></returns>
        public static YamlStream ToYaml(this string value)
        {
            var resultYaml = new YamlStream();
            resultYaml.Load(new StringReader(value));
            return resultYaml;
        }
    }
}
