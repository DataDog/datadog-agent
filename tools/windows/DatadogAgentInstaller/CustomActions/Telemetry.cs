using System;
using Microsoft.Deployment.WindowsInstaller;
using System.Collections.Generic;
using System.IO;
using System.Reflection;

namespace Datadog.CustomActions
{
    public class Telemetry
    {
        private readonly IInstallerHttpClient _client;
        private readonly ISession _session;

        public const string DefaultSite = "datadoghq.com";

        public Telemetry(
            IInstallerHttpClient client,
            ISession session)
        {
            _client = client;
            _session = session;
        }

        public static Telemetry DefaultTelemetry(ISession session)
        {
            return new Telemetry(new InstallerWebClient(), session);
        }

        public void ReportTelemetry(string eventName)
        {
            var apikey = _session["APIKEY"];
            var site = _session["SITE"];
            if (string.IsNullOrEmpty(site))
            {
                site = DefaultSite;
            }
            var uri = $"https://instrumentation-telemetry-intake.{site}/api/v2/apmtelemetry";

            var installerVersion = Assembly.GetExecutingAssembly().GetName().Version;
            var agentVersion = CiInfo.PackageVersion;
            var payload = @$"
{{
    ""request_type"": ""apm-onboarding-event"",
    ""api_version"": ""v1"",
    ""payload"": {{
        ""event_name"": ""{eventName}"",
        ""tags"": {{
            ""agent_platform"": ""native"",
            ""agent_version"": ""{ agentVersion}"",
            ""script_version"": ""{ installerVersion}""
        }}
    }}
}}";

            _client.Post(uri, payload, new Dictionary<string, string>
            {
                { "DD-Api-Key", apikey }, 
                { "Content-Type", "application/json" },
            });
        }

        private static ActionResult Report(ISession session)
        {
            try
            {
                var telemetry = DefaultTelemetry(session);
                var wixLogLocation = session["MsiLogFileLocation"];
                if (!string.IsNullOrEmpty(wixLogLocation) &&
                    File.Exists(wixLogLocation))
                {
                    var log = File.ReadAllText(wixLogLocation);
                    if (log.Contains("Product: Datadog Agent -- Installation failed."))
                    {
                        telemetry.ReportTelemetry("agent.installation.error");
                    }
                    else
                    {
                        telemetry.ReportTelemetry("agent.installation.success");
                    }
                }
            }
            catch (Exception e)
            {
                session.Log($"Error sending telemetry: {e}");
                return ActionResult.Failure;
            }
            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult Report(Session session)
        {
            return Report(new SessionWrapper(session));
        }
    }
}
