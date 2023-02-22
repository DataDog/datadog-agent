using System;
using System.Collections.Generic;
using System.IO;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions
{
    public class Flare
    {
        private readonly IInstallerHttpClient _client;
        private readonly ISession _session;

        public const string DefaultSite = "datadoghq.com";

        public Flare(
            IInstallerHttpClient client,
            ISession session)
        {
            _client = client;
            _session = session;
        }

        public static Flare DefaultFlare(ISession session)
        {
            return new Flare(new InstallerWebClient(), session);
        }

        public void Send()
        {
            var agentVersion = CiInfo.PackageVersion;
            var apikey = _session["APIKEY"];
            var email = _session["EMAIL"];
            var wixLogLocation = _session["MsiLogFileLocation"];
            var log = "";
            try
            {
                if (string.IsNullOrEmpty(wixLogLocation))
                {
                    _session.Log("Property MsiLogFileLocation is empty");
                }
                else if (File.Exists(wixLogLocation))
                {
                    log = File.ReadAllText(wixLogLocation);
                }
                else
                {
                    _session.Log($"Log file \"{wixLogLocation}\" does not exist");
                }
            }
            catch (Exception e)
            {
                _session.Log($"Error retrieving install logs: {e}");
            }

            var site = _session["SITE"];
            if (string.IsNullOrEmpty(site))
            {
                site = DefaultSite;
            }
            var uri = $"https://api.{site}/agent_stats/report_failure";
            var payload = $"os=Windows&version={agentVersion}&log={log}&email={email}&apikey={apikey}";
            _client.Post(uri, Uri.EscapeDataString(payload), new Dictionary<string, string>
            {
                { "Content-Type", "application/x-www-form-urlencoded" }
            });
        }

        private static ActionResult SendFlare(ISession session)
        {
            try
            {
                DefaultFlare(session).Send();
            }
            catch (Exception e)
            {
                session.Log($"Error sending flare: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult SendFlare(Session session)
        {
            return SendFlare(new SessionWrapper(session));
        }
    }
}
