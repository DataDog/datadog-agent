using System.Collections.Generic;

namespace Datadog.CustomActions
{
    /// <summary>
    /// Interface for an HTTP client.
    /// </summary>
    /// This will make it easier for unit testing HTTP dependent code,
    /// and also migration to modern HTTP libraries easier once we get
    /// rid of .Net 3.5/Windows Server 2008.
    public interface IInstallerHttpClient
    {
        void Post(string uri, string payload, Dictionary<string, string> headers);
    }
}
