using System;
using System.IO;
using Datadog.CustomActions.Extensions;
using Microsoft.Deployment.WindowsInstaller;
using YamlDotNet.Serialization.NamingConventions;
using YamlDotNet.Serialization;
using System.Collections.Generic;
using System.Text.RegularExpressions;
using System.Text;

namespace Datadog.CustomActions
{

    public class ConfigCustomActions
    {
        class DatadogConfig
        {
            public string ApiKey { get; set; }
            public string Site { get; set; }
        }

        private static ActionResult ReadConfig(ISession session)
        {
            var configFolder = session.Property("APPLICATIONDATADIRECTORY");
            try
            {
                using (var input = new StreamReader(Path.Combine(configFolder, "datadog.yaml")))
                {
                    var deserializer = new DeserializerBuilder()
                        .IgnoreUnmatchedProperties()
                        .WithNamingConvention(UnderscoredNamingConvention
                            .Instance) // see height_in_inches in sample yml 
                        .Build();

                    var datadogConfig = deserializer.Deserialize<DatadogConfig>(input);
                    if (string.IsNullOrEmpty(session["APIKEY"]))
                    {
                        session["APIKEY"] = datadogConfig.ApiKey;
                    }

                    if (string.IsNullOrEmpty(session["SITE"]))
                    {
                        session["SITE"] = datadogConfig.Site;
                    }
                }
            }
            catch (Exception e)
            {
                session.Log($"{nameof(ReadConfig)}: User config could not be read: ", e);
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ReadConfig(Session session)
        {
            return ReadConfig(new SessionWrapper(session));
        }

        public interface IReplacer
        {
            string PropertyName { get; }
            bool IsMatch(string input);
            string Replace(string input, string propertyValue);
        }

        public class FormatValueReplacer : IReplacer
        {
            private readonly Func<string, string> _replacement;
            private readonly string _regex;
            private Match _match;

            public FormatValueReplacer(string propertyName, string regex, Func<string, string> replacement)
            {
                PropertyName = propertyName;
                _replacement = replacement;
                _regex = regex;
            }

            public string PropertyName { get; }

            public bool IsMatch(string input)
            {
                _match = Regex.Match(input, _regex, RegexOptions.Multiline);
                return _match != null && _match.Success;
            }

            public string Replace(string input, string propertyValue)
            {
                return Regex.Replace(input, _regex, _replacement(propertyValue), RegexOptions.Multiline | RegexOptions.ECMAScript);
            }
        }

        static string FormatProxy(string value, ISession session)
        {
            var hostUri = new UriBuilder(value);
            var proxyConfiguration = new StringBuilder();

            if (!string.IsNullOrEmpty(session.Property("PROXY_USER")))
            {
                hostUri.UserName = session.Property("PROXY_USER");
                if (!string.IsNullOrEmpty(session.Property("PROXY_PASSWORD")))
                {
                    hostUri.Password = session.Property("PROXY_PASSWORD");
                }
            }

            if (!string.IsNullOrEmpty(session.Property("PROXY_PORT")))
            {
                if (!int.TryParse(session.Property("PROXY_PORT"), out var port))
                {
                    throw new Exception("Invalid PROXY_PORT");
                }
                hostUri.Port = port;
            }

            proxyConfiguration.AppendLine("proxy:");
            proxyConfiguration.AppendLine($"  https: {hostUri}");
            proxyConfiguration.AppendLine($"  http: {hostUri}");

            return proxyConfiguration.ToString();
        }

        static string FormatTags(string value)
        {
            const string newTagLine = "\n  - ";
            var tagsConfiguration = new StringBuilder();
            tagsConfiguration.Append("tags: ");
            tagsConfiguration.Append(newTagLine);
            tagsConfiguration.Append(string.Join(newTagLine, value.Split(',')));
            return tagsConfiguration.ToString();
        }

        public static string ReplaceProperties(string yaml, ISession session)
        {
            var replacers = new List<IReplacer>
            {
                new FormatValueReplacer("APIKEY",                           "^[ #]*api_key:.*", (value) => $"api_key: {value}"),
                new FormatValueReplacer("SITE",                             "^[ #]*site:.*", (value) => $"site: {value}"),
                new FormatValueReplacer("HOSTNAME",                         "^[ #]*hostname:.*", (value) => $"hostname: {value}"),
                new FormatValueReplacer("LOGS_ENABLED",                     "^[ #]*logs_config:.*", (value) => "logs_config:"),
                new FormatValueReplacer("LOGS_ENABLED",                     "^[ #]*logs_enabled:.*", (value) => $"logs_enabled: {value}"),
                new FormatValueReplacer("LOGS_DD_URL",                      "^[ #]*logs_config:.*", (value) => "logs_config:"),
                new FormatValueReplacer("LOGS_DD_URL",                      "^[ #]*logs_dd_url:.*", (value) => $"  logs_dd_url: {value}"),
                new FormatValueReplacer("PROCESS_ENABLED",                  "^[ #]*process_config:[ \n\r#]*process_collection:[ \n\r#]*enabled:.*", (value) => $"process_config:\n  process_collection:\n    enabled: {value}"),

                new FormatValueReplacer("PROCESS_DD_URL",                   "^[ #]*process_config:.*", (value) => $"process_config:\n  process_dd_url: {value}"),

                // Uncomment the process_config section in case PROCESS_DD_URL was not specified
                new FormatValueReplacer("PROCESS_DISCOVERY_ENABLED",        "^[ #]*process_config:.*", (value) => "process_config:"),
                new FormatValueReplacer("PROCESS_DISCOVERY_ENABLED",        "^[ #]*process_discovery:[ \n\r#]*enabled:.*", (value) => $"  process_discovery:\n    enabled: {value}"),

                new FormatValueReplacer("APM_ENABLED",                      "^[ #]*apm_config:[ \n\r#]*enabled:.*", (value) => $"apm_config:\n  enabled: {value}"),

                // Uncomment the apm_config in case APM_ENABLED was not specified
                new FormatValueReplacer("TRACE_DD_URL",                     "^[ #]*apm_config:", (value) => "apm_config:"),
                new FormatValueReplacer("TRACE_DD_URL",                     "^[ #]*apm_dd_url:.*", (value) => $"  apm_dd_url: {value}"),

                // There are multiple cmd_port entry in the config, so we need
                // to match the top level one explicitly.
                new FormatValueReplacer("CMD_PORT",                         "^[# ]?[ ]*cmd_port:.*", (value) => $"cmd_port: {value}"),
                new FormatValueReplacer("DD_URL",                           "^[ #]*dd_url:.*", (value) => $"dd_url: {value}"),
                new FormatValueReplacer("PYVER",                            "^[ #]*python_version:.*", (value) => $"python_version: {value}"),
                new FormatValueReplacer("PROXY_HOST",                       "^[ #]*proxy:.*", (value) => FormatProxy(value, session)),
                new FormatValueReplacer("HOSTNAME_FQDN_ENABLED",            "^[ #]*hostname_fqdn:.*", (value) => $"hostname_fqdn: {value}"),
                new FormatValueReplacer("TAGS",                             "^[ #]*tags:(?:(?:.|\n)*?)^[ #]*- <TAG_KEY>:<TAG_VALUE>", FormatTags),
                new FormatValueReplacer("EC2_USE_WINDOWS_PREFIX_DETECTION", "^[ #]*ec2_use_windows_prefix_detection:.*", (value) => $"ec2_use_windows_prefix_detection: {value}"),
            };

            foreach (var replacer in replacers)
            {
                if (!string.IsNullOrEmpty(session.Property(replacer.PropertyName)))
                {
                    if (!replacer.IsMatch(yaml))
                    {
                        session.Log($"{replacer.PropertyName} did not match");
                        continue;
                    }
                    yaml = replacer.Replace(yaml, session.Property(replacer.PropertyName));
                }
            }

            return yaml;
        }

        private static ActionResult WriteConfig(ISession session)
        {
            var configFolder = session.Property("APPLICATIONDATADIRECTORY");

            try
            {
                string yaml;
                using (var input = new StreamReader(Path.Combine(configFolder, "datadog.yaml")))
                {
                    yaml = input.ReadToEnd();
                }

                yaml = ReplaceProperties(yaml, session);

                using (var output = new StreamWriter("test.yaml"))
                {
                    output.Write(yaml);
                }

            }
            catch (Exception e)
            {
                session.Log($"{nameof(WriteConfig)}: User config could not be written: ", e);
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult WriteConfig(Session session)
        {
            return WriteConfig(new SessionWrapper(session));
        }
    }
}
