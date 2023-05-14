using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text;
using System.Xml.Linq;
using ICSharpCode.SharpZipLib.Tar;
using SevenZip;
using WixSharp;
using File = System.IO.File;

namespace WixSetup
{
    public class CompressedDir : WixSharp.File
    {
        private readonly string _sourceDir;

        public CompressedDir(IWixProjectEvents wixProjectEvents, string targetPath, string sourceDir)
            : base($"{targetPath}.COMPRESSED")
        {
            _sourceDir = sourceDir;
            wixProjectEvents.WixSourceGenerated += OnWixSourceGenerated;
        }

        public void OnWixSourceGenerated(XDocument document)
        {
#if DEBUG
            // In debug mode, skip generating the file if it
            // already exists. Delete the file to regenerate it.
            if (File.Exists(Name))
            {
                return;
            }
#endif
            var tar = $"{Name}.tar";

            var filesInSourceDir = new DirectoryInfo(_sourceDir)
                .EnumerateFiles("*", SearchOption.AllDirectories)
                .ToArray();
            var sourceDirName = Path.GetFileName(_sourceDir);
            var directorySize = filesInSourceDir
                .Sum(file => file.Length)
                .ToString();
            document
                .Select("Wix/Product")
                .AddElement("Property", $"Id={sourceDirName}_SIZE; Value={directorySize}");

            using (var outStream = File.Create(tar))
            {
                using var tarArchive = new TarOutputStream(outStream, Encoding.UTF8);
                foreach (var file in filesInSourceDir)
                {
                    // Path in tar must be in UNIX format
                    var nameInTar = $"{sourceDirName}{file.FullName.Substring(_sourceDir.Length)}".Replace('\\', '/');
                    var entry = TarEntry.CreateTarEntry(nameInTar);
                    using var fileStream = File.OpenRead(file.FullName);
                    entry.Size = fileStream.Length;
                    tarArchive.PutNextEntry(entry);
                    fileStream.CopyTo(tarArchive);
                    tarArchive.CloseEntry();
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
