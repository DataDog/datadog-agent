using System;
using System.Collections.Generic;
using System.IO;
using System.Text;
using System.Windows.Forms;
using Datadog.CustomActions.Interfaces;
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

        public void Send(string email)
        {
            var agentVersion = CiInfo.PackageVersion;
            var apikey = _session["APIKEY"];
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
                    using var fs = new FileStream(wixLogLocation, FileMode.Open, FileAccess.Read, FileShare.ReadWrite);
                    using var sr = new StreamReader(fs, Encoding.Default);
                    log = sr.ReadToEnd();
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
            var payload = new System.Collections.Specialized.NameValueCollection
            {
                { "os", "Windows" },
                { "version", agentVersion },
                { "log", log },
                { "email", email },
                { "apikey", apikey },
                { "variant", "windows_installer" }
            };

            _client.Post(uri, payload, new Dictionary<string, string>());
        }

        private static void SendFlare(ISession session)
        {
            try
            {
                DefaultFlare(session).Send(session["EMAIL"]);
                MessageBox.Show(@"Flare sent successfully.", @"Flare", MessageBoxButtons.OK);
            }
            catch (Exception e)
            {
                session.Log($"Error sending flare: {e}");
                MessageBox.Show(@$"Failed to send the flare: {e.Message}", @"Flare", MessageBoxButtons.OK);
            }
        }

        [CustomAction]
        public static ActionResult SendFlare(Session session)
        {
            if (int.Parse(session["UILevel"]) > 3)
            {
                SendFlare(new SessionWrapper(session));
            }
            // Never ever return failure here, or the installer will abruptly stop.
            return ActionResult.Success;
        }
    }
}
