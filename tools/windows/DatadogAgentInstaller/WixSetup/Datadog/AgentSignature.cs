using System;
using System.Collections.Generic;
using WixSharp;

namespace WixSetup.Datadog
{
    public class AgentSignature
    {
        public DigitalSignature Signature { get; }

        public AgentSignature()
        {
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
    }
}
