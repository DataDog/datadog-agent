using System;
using System.Collections.Generic;
using System.Collections.Specialized;
using System.Net;
using System.Security.Authentication;

namespace Datadog.CustomActions.Interfaces
{
    public class InstallerWebClient : IInstallerHttpClient
    {
        private const SslProtocols _Tls12 = (SslProtocols)0x00000C00;
        private const SecurityProtocolType Tls12 = (SecurityProtocolType)_Tls12;

        public InstallerWebClient()
        {
            ServicePointManager.SecurityProtocol = Tls12;
        }

        private static void SetHeaders(WebClient webClient, Dictionary<string, string> headers)
        {
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
        }

        public void Post(string uri, string payload, Dictionary<string, string> headers)
        {
            var webClient = new WebClient();
            SetHeaders(webClient, headers);

            webClient.UploadString(new Uri(uri), payload);
        }

        public void Post(string uri, NameValueCollection payload, Dictionary<string, string> headers)
        {
            var webClient = new WebClient();
            SetHeaders(webClient, headers);
            webClient.Headers[HttpRequestHeader.ContentType] = "application/x-www-form-urlencoded";
            webClient.UploadValues(uri, payload);
        }
    }
}
