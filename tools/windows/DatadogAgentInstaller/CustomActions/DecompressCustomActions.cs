using Microsoft.Deployment.WindowsInstaller;
using System;
using System.IO;
using Cave.Compression.Tar;
using Datadog.CustomActions.Extensions;
using System.Text;

namespace Datadog.CustomActions
{
    public class DecompressCustomActions
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

        private static ActionResult DecompressPythonDistributions(ISession session)
        {
            var projectLocation = session.Property("PROJECTLOCATION");

            try
            {
                var embedded2 = Path.Combine(projectLocation, "embedded2.COMPRESSED");
                if (File.Exists(embedded2))
                {
                    Decompress(embedded2);
                }
                else
                {
                    session.Log($"{nameof(DecompressPythonDistributions)}: {embedded2} not found, skipping decompression.");
                }
            }
            catch (Exception e)
            {
                session.Log($"{nameof(DecompressPythonDistributions)}: Error while decompressing embedded2.COMPRESSED: {e}");
                return ActionResult.Failure;
            }


            try
            {
                var embedded3 = Path.Combine(projectLocation, "embedded3.COMPRESSED");
                if (File.Exists(embedded3))
                {
                    Decompress(embedded3);
                }
                else
                {
                    session.Log($"{nameof(DecompressPythonDistributions)}: {embedded3} not found, skipping decompression.");
                }
            }
            catch (Exception e)
            {
                session.Log($"{nameof(DecompressPythonDistributions)}: Error while decompressing embedded3.COMPRESSED: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult DecompressPythonDistributions(Session session)
        {
            return DecompressPythonDistributions(new SessionWrapper(session));
        }
    }
}
