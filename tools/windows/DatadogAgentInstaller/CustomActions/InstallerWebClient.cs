using System;
using System.Collections.Generic;
using System.Net;

namespace Datadog.CustomActions
{
    public class InstallerWebClient : IInstallerHttpClient
    {
        public void Post(string uri, string payload, Dictionary<string, string> headers)
        {
            var webClient = new WebClient();
            foreach (var header in headers)
            {
                if (webClient.Headers.Get(header.Key) != null)
                {
                    webClient.Headers[header.Key] = header.Value;
                }
                else
                {
                    webClient.Headers.Add(header.Key, header.Value);
                }
            }
            
            webClient.UploadString(new Uri(uri), payload);
        }
    }
}
