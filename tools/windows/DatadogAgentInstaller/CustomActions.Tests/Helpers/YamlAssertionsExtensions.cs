using System.Linq;
using YamlDotNet.RepresentationModel;

namespace CustomActions.Tests.Helpers
{
    /// <summary>
    /// Helper class to convert a YAML stream into a YAML assertion object.
    /// </summary>
    public static class YamlAssertionsExtensions
    {
        /// <summary>
        /// Converts a <see cref="YamlStream"/> into a <see cref="YamlAssertions"/> object.
        /// This allows running assertions on a YAML stream.
        /// </summary>
        /// <param name="instance">The instance to convert.</param>
        /// <returns>A <see cref="YamlAssertions"/>object.</returns>
        public static YamlAssertions Should(this YamlStream instance)
        {
            // If the doc is empty, don't fail but return an assertion over an empty doc.
            var doc = instance.Documents.FirstOrDefault() ?? new YamlDocument(new YamlMappingNode());
            return new YamlAssertions(doc.RootNode);
        }
    }
}
