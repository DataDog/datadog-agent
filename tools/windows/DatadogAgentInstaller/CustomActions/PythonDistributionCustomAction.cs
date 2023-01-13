using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Collections.Generic;
using System.IO;
using Cave.Compression.Tar;
using Datadog.CustomActions.Extensions;
using System.Text;

namespace Datadog.CustomActions
{
    public class PythonDistributionCustomAction
    {
        static void Decompress(string compressedFileName)
        {
            var decoder = new SevenZip.Compression.LZMA.Decoder();
            using (var inStream = File.OpenRead(compressedFileName))
            {
                using (var outStream = File.Create($"{compressedFileName}.tar"))
                {
                    var reader = new BinaryReader(inStream, Encoding.UTF8);
                    // Properties of the stream are encoded on 5 bytes
                    var props = reader.ReadBytes(5);
                    decoder.SetDecoderProperties(props);
                    var length = reader.ReadInt64();
                    decoder.Code(inStream, outStream, inStream.Length, length, null);
                    outStream.Flush();
                }
            }


            var outputPath = Path.GetDirectoryName(Path.GetFullPath(compressedFileName));
            using (var inStream = File.OpenRead($"{compressedFileName}.tar"))
            using (var tarInStream = new TarReader(inStream, false))
            {
                tarInStream.UnpackTo(outputPath, null, null);
            }
            File.Delete($"{compressedFileName}.tar");
            File.Delete($"{compressedFileName}");
        }

        private static ActionResult DecompressPythonDistribution(ISession session, string compressedDisitributionFile)
        {
            var projectLocation = session.Property("PROJECTLOCATION");

            try
            {
                var embedded = Path.Combine(projectLocation, compressedDisitributionFile);
                if (File.Exists(embedded))
                {
                    Decompress(embedded);
                }
                else
                {
                    if (embedded.Contains("embedded3"))
                    {
                        throw new InvalidOperationException($"The file {embedded} doesn't exist, but it should");
                        
                    }
                    session.Log($"{embedded} not found, skipping decompression.");
                }
            }
            catch (Exception e)
            {
                session.Log($"Error while decompressing {compressedDisitributionFile}: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        private static ActionResult DecompressPythonDistributions(ISession session)
        {
            var actionResult = DecompressPythonDistribution(session, "embedded2.COMPRESSED");
            if (actionResult != ActionResult.Success)
            {
                return actionResult;
            }
            return DecompressPythonDistribution(session, "embedded3.COMPRESSED");
        }

        [CustomAction]
        public static ActionResult DecompressPythonDistributions(Session session)
        {
            return DecompressPythonDistributions(new SessionWrapper(session));
        }
    }
}
