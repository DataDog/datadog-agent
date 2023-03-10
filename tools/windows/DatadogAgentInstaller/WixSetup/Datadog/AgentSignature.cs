using System;
using System.Collections.Generic;
using System.Xml.Linq;
using WixSharp;

namespace WixSetup.Datadog
{
    public class AgentSignature
    {
        private readonly AgentPython _agentPython;
        private readonly AgentBinaries _agentBinaries;

        public DigitalSignature Signature { get; }

        public AgentSignature(IWixProjectEvents wixProjectEvents, AgentPython agentPython, AgentBinaries agentBinaries)
        {
            wixProjectEvents.WixSourceGenerated += OnWixSourceGenerated;
            _agentPython = agentPython;
            _agentBinaries = agentBinaries;
            var pfxFilePath = Environment.GetEnvironmentVariable("SIGN_PFX");
            var pfxFilePassword = Environment.GetEnvironmentVariable("SIGN_PFX_PW");

            if (pfxFilePath != null && pfxFilePassword != null)
            {
                Signature = new DigitalSignature
                {
                    PfxFilePath = pfxFilePath,
                    Password = pfxFilePassword,
                    HashAlgorithm = HashAlgorithmType.sha256,
                    // Only use timestamp servers from Microsoft-approved authenticode providers
                    // See https://docs.microsoft.com/en-us/windows/win32/seccrypto/time-stamping-authenticode-signatures
                    TimeUrls = new List<Uri>
                    {
                        new Uri("http://timestamp.digicert.com"),
                        new Uri("http://timestamp.globalsign.com/scripts/timstamp.dll"),
                        new Uri("http://timestamp.comodoca.com/authenticode"),
                        new Uri("http://www.startssl.com/timestamp"),
                        new Uri("http://timestamp.sectigo.com"),
                    }
                };
            }
        }

        public void OnWixSourceGenerated(XDocument document)
        {
            if (Signature != null)
            {
                var filesToSign = new List<string>
                {
                    _agentBinaries.Agent,
                    _agentBinaries.Tray,
                    _agentBinaries.ProcessAgent,
                    _agentBinaries.SystemProbe,
                    _agentBinaries.TraceAgent,
                    _agentBinaries.LibDatadogAgentThree
                };
                filesToSign.AddRange(_agentBinaries.PythonThreeBinaries);
                if (_agentPython.IncludePython2)
                {
                    filesToSign.Add(_agentBinaries.LibDatadogAgentTwo);
                    filesToSign.AddRange(_agentBinaries.PythonTwoBinaries);
                }

                foreach (var file in filesToSign)
                {
                    Signature.Apply(file);
                }
            }
        }
    }
}
