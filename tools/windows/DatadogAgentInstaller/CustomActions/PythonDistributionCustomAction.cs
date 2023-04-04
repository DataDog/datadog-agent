using Microsoft.Deployment.WindowsInstaller;
using System;
using System.IO;
using Datadog.CustomActions.Extensions;
using System.Text;
using Datadog.CustomActions.Interfaces;
using ICSharpCode.SharpZipLib.Tar;

namespace Datadog.CustomActions
{
    public class PythonDistributionCustomAction
    {
        public static class MessageRecordFields
        {
            /// <summary>
            /// Resets progress bar and sets the expected total number of ticks in the bar.
            /// </summary>
            public const int MasterReset = 0;

            /// <summary>
            /// Provides information related to progress messages to be sent by the current action.
            /// </summary>
            public const int ActionInfo = 1;

            /// <summary>
            /// Increments the progress bar.
            /// </summary>
            public const int ProgressReport = 1;

            /// <summary>
            /// Enables an action (such as CustomAction) to add ticks to the expected total number of progress of the progress bar.
            /// </summary>
            public const int ProgressAddition = 1;
        }

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
            using (var tarArchive = TarArchive.CreateInputTarArchive(inStream, Encoding.UTF8))
            {
                tarArchive.ExtractContents(outputPath);
            }
            File.Delete($"{compressedFileName}.tar");
            File.Delete($"{compressedFileName}");
        }

        private static ActionResult DecompressPythonDistribution(
            ISession session,
            string compressedDistributionFile,
            string pythonDistributionName,
            int pythonDistributionSize)
        {
            var projectLocation = session.Property("PROJECTLOCATION");

            try
            {
                var embedded = Path.Combine(projectLocation, compressedDistributionFile);

                if (File.Exists(embedded))
                {
                    using var actionRecord = new Record(
                        "Decompress Python distribution",
                        $"Decompressing {pythonDistributionName} distribution",
                        ""
                    );
                    session.Message(InstallMessage.ActionStart, actionRecord);

                    {
                        using var record = new Record(MessageRecordFields.ActionInfo,
                            0,  // Number of ticks the progress bar moves for each ActionData message. This field is ignored if Field 3 is 0.
                            0   // The current action will send explicit ProgressReport messages.
                            );
                        session.Message(InstallMessage.Progress, record);
                    }
                    Decompress(embedded);
                    {
                        using var record = new Record(MessageRecordFields.ProgressReport, pythonDistributionSize);
                        session.Message(InstallMessage.Progress, record);
                    }
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
                session.Log($"Error while decompressing {compressedDistributionFile}: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        private static ActionResult DecompressPythonDistributions(ISession session)
        {
            int size = 0;
            var embedded2Size = session.Property("embedded2_SIZE");
            if (!string.IsNullOrEmpty(embedded2Size))
            {
                size = int.Parse(embedded2Size);
            }
            var actionResult = DecompressPythonDistribution(session, "embedded2.COMPRESSED", "Python 2", size);
            if (actionResult != ActionResult.Success)
            {
                return actionResult;
            }
            var embedded3Size = session.Property("embedded3_SIZE");
            if (!string.IsNullOrEmpty(embedded3Size))
            {
                size  = int.Parse(embedded3Size);
            }
            return DecompressPythonDistribution(session, "embedded3.COMPRESSED", "Python 3", size);
        }

        [CustomAction]
        public static ActionResult DecompressPythonDistributions(Session session)
        {
            return DecompressPythonDistributions(new SessionWrapper(session));
        }

        private static ActionResult PrepareDecompressPythonDistributions(ISession session)
        {
            try
            {
                int total = 0;
                var embedded2Size = session.Property("embedded2_SIZE");
                if (!string.IsNullOrEmpty(embedded2Size))
                {
                    total += int.Parse(embedded2Size);
                }
                var embedded3Size = session.Property("embedded3_SIZE");
                if (!string.IsNullOrEmpty(embedded3Size))
                {
                    total += int.Parse(embedded3Size);
                }
                // Add embedded Python size to the progress bar size
                // Even though we can't record accurate progress, it will look like it's
                // moving every time we decompress a Python distribution.
                using var record = new Record(MessageRecordFields.ProgressAddition, total);
                if (session.Message(InstallMessage.Progress, record) != MessageResult.OK)
                {
                    session.Log("Could not set the progress bar size");
                    return ActionResult.Failure;
                }
            }
            catch (Exception e)
            {
                session.Log($"Error settings the progress bar size: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult PrepareDecompressPythonDistributions(Session session)
        {
            return PrepareDecompressPythonDistributions(new SessionWrapper(session));
        }
    }
}
