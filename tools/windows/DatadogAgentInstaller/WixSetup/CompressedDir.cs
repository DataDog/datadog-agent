using System.IO;
using System.Xml.Linq;
using Cave.Compression.Tar;

namespace WixSetup
{
    public class CompressedDir : WixSharp.File
    {
        private readonly string _sourceDir;

        public CompressedDir(IWixProjectEvents wixProjectEvents, string targetPath, string sourceDir)
            : base($"{targetPath}.tar.gz")
        {
            wixProjectEvents.WixSourceGenerated += OnWixSourceGenerated;
            _sourceDir = sourceDir;
        }

        public void OnWixSourceGenerated(XDocument document)
        {
            using (var outStream = File.Create(Name))
            {
                using (var writer = new TarWriter(outStream, false))
                {
                    writer.AddDirectory(Path.GetFileName(_sourceDir), _sourceDir);
                }
            }
        }
    }
}
