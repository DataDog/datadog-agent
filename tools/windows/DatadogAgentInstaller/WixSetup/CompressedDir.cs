using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text;
using System.Xml.Linq;
using Cave.Compression.Tar;
using Datadog.CustomActions;
using SevenZip;
using WixSharp;
using File = System.IO.File;

namespace WixSetup
{
    public class CompressedDir : WixSharp.File
    {
        private readonly string _sourceDir;

        public CompressedDir(IWixProjectEvents wixProjectEvents, string targetPath, string sourceDir)
            : base($"{targetPath}.COMPRESSED", new ManagedAction(UserCustomActions.ProcessDdAgentUserCredentials))
        {
            wixProjectEvents.WixSourceGenerated += OnWixSourceGenerated;
            _sourceDir = sourceDir;
        }

        public void OnWixSourceGenerated(XDocument document)
        {
            var tar = $"{Name}.tar";

            using (var outStream = File.Create(tar))
            {
                using (var tarOutStream = new TarWriter(outStream, false))
                {
                    tarOutStream.AddDirectory(Path.GetFileName(_sourceDir), _sourceDir);
                }
            }

            using (var inStream = File.Open(tar, FileMode.Open))
            using (var outStream = File.Create(Name))
            {
                Compress(inStream, outStream);
            }
            File.Delete(tar);
        }

        static void Compress(Stream inStream, Stream outStream)
        {
            var encoder = new SevenZip.Compression.LZMA.Encoder();
            var encodingProps = new Dictionary<CoderPropID, object>
            {
                {CoderPropID.DictionarySize, 32 * 1024 * 1024},
                {CoderPropID.PosStateBits,   2},
                {CoderPropID.LitContextBits, 3},
                {CoderPropID.LitPosBits,     0},
                {CoderPropID.Algorithm,      2},
                {CoderPropID.NumFastBytes,   64},
                {CoderPropID.MatchFinder,    "bt4"}
            };

            encoder.SetCoderProperties(encodingProps.Keys.ToArray(), encodingProps.Values.ToArray());
            encoder.WriteCoderProperties(outStream);
            var writer = new BinaryWriter(outStream, Encoding.UTF8);
            writer.Write(inStream.Length - inStream.Position);
            encoder.Code(inStream, outStream, -1, -1, null);
        }
    }
}
