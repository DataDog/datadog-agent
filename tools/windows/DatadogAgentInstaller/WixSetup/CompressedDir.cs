using System;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Xml.Linq;
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

        private void OnWixSourceGenerated(XDocument document)
        {
#if DEBUG
            // In debug mode, skip generating the file if it
            // already exists. Delete the file to regenerate it.
            if (File.Exists(Name))
            {
                return;
            }
#endif

            FileInfo[] filesInSourceDir = new DirectoryInfo(_sourceDir)
                .EnumerateFiles("*", SearchOption.AllDirectories)
                .ToArray();
            var sourceDirName = Path.GetFileName(_sourceDir);
            var directorySize = filesInSourceDir
                .Sum(file => file.Length)
                .ToString();
            document
                .Select("Wix/Product")
                .AddElement("Property", $"Id={sourceDirName}_SIZE; Value={directorySize}");

            var psi = new ProcessStartInfo
            {
                CreateNoWindow = true,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                UseShellExecute = false,
                FileName = "7z.exe",
                Arguments = $"a -t7z {Name} \"{_sourceDir}\" -mx=5 -ms=on"
            };
            var process = new Process();
            process.StartInfo = psi;
            process.EnableRaisingEvents = true;
            process.OutputDataReceived += (_, args) => Console.WriteLine(args.Data);
            process.ErrorDataReceived += (_, args) => Console.Error.WriteLine(args.Data);
            process.Start();
            process.BeginOutputReadLine();
            process.BeginErrorReadLine();
            process.WaitForExit();
            if (process.ExitCode != 0)
            {
                throw new Exception($"7z failed with code {process.ExitCode}");
            }
        }
    }
}
