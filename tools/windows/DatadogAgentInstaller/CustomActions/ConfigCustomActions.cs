using System;
using System.IO;
using Datadog.CustomActions.Extensions;
using Microsoft.Deployment.WindowsInstaller;
using YamlDotNet.Serialization.NamingConventions;
using YamlDotNet.Serialization;
using System.Collections.Generic;
using System.Linq;
using System.Text.RegularExpressions;
using System.Text;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions
{
    public class ConfigCustomActions
    {
        /// <summary>
        /// Subset of the Datadog config file that we are going to read.
        /// </summary>
        // ReSharper disable once ArrangeTypeMemberModifiers
        class DatadogConfig
        {
            // ReSharper disable once UnusedAutoPropertyAccessor.Local
            public string ApiKey { get; set; }
            // ReSharper disable once UnusedAutoPropertyAccessor.Local
            public string Site { get; set; }
        }

        private static ActionResult ReadConfig(ISession session)
        {
            try
            {
                var configFolder = session.Property("APPLICATIONDATADIRECTORY");
                var configFilePath = Path.Combine(configFolder, "datadog.yaml");
                if (!File.Exists(configFilePath))
                {
                    session.Log($"No user config found in {configFilePath}, continuing.");
                    session["DATADOGYAMLEXISTS"] = "no";
                    return ActionResult.Success;
                }

                session["DATADOGYAMLEXISTS"] = "yes";

                using var input = new StreamReader(configFilePath);
                var deserializer = new DeserializerBuilder()
                    .IgnoreUnmatchedProperties()
                    .WithNamingConvention(UnderscoredNamingConvention.Instance)
                    .Build();

                var datadogConfig = deserializer.Deserialize<DatadogConfig>(input);
                if (string.IsNullOrEmpty(session.Property("APIKEY")))
                {
                    session["APIKEY"] = datadogConfig.ApiKey;
                }

                if (string.IsNullOrEmpty(session.Property("SITE")))
                {
                    session["SITE"] = datadogConfig.Site;
                }
            }
            catch (Exception e)
            {
                session.Log($"User config could not be read: {e}");
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ReadConfig(Session session)
        {
            return ReadConfig(new SessionWrapper(session));
        }

        /// <summary>
        /// Class to help do search and replace in a configuration file.
        /// </summary>
        /// Given a property name, it matches a pattern and replaces it with some text.
        public class PropertyReplacer
        {
            /// <summary>
            /// Internal helper class that simply groups a Regex with a replacement function.
            /// </summary>
            /// Necessary because tuples are not available in .Net 3.5.
            class RegexWithReplacer
            {
                public Regex Regex { get; set; }
                public Func<string, string> Replacer { get; set; }
            }

            /// <summary>
            /// A list of regex+replacer in the order they are to be searched (first to last).
            /// </summary>
            private readonly List<RegexWithReplacer> _regexReplacersInOrder = new List<RegexWithReplacer>();

            /// <summary>
            /// Create a new <see cref="PropertyReplacer"/>
            /// </summary>
            /// <param name="property">The property name.</param>
            /// <returns>A <see cref="PropertyReplacer"/></returns>
            public static PropertyReplacer For(string property)
            {
                return new PropertyReplacer(property);
            }

            /// <summary>
            /// Adds a pattern to match for the given <see cref="PropertyName"/>.
            /// </summary>
            /// <param name="pattern">The regex pattern to match.</param>
            /// <returns>This object.</returns>
            public PropertyReplacer Match(string pattern)
            {
                _regexReplacersInOrder.Add(new RegexWithReplacer
                {
                    Regex = new Regex(pattern, RegexOptions.Multiline)
                });
                return this;
            }

            /// <summary>
            /// Adds a replacer function for the *last* pattern recorded.
            /// </summary>
            /// The replacer function takes in the property value, as evaluated
            /// at runtime, and returns a string that will be substituted in the source text.
            /// <param name="replacer">The replacer function.</param>
            /// <returns>This object.</returns>
            /// <exception cref="Exception">Throws an exception if there are no patterns to associate
            /// with this replacer.</exception>
            public PropertyReplacer ReplaceWith(Func<string, string> replacer)
            {
                if (_regexReplacersInOrder.Count == 0)
                {
                    throw new Exception("Cannot use ReplaceWith without a Match");
                }
                _regexReplacersInOrder.Last().Replacer = replacer;
                return this;
            }

            private PropertyReplacer(string propertyName)
            {
                PropertyName = propertyName;
            }

            /// <summary>
            /// The property associated with this object.
            /// </summary>
            public string PropertyName { get; }

            /// <summary>
            /// Check if *all* the regex match, in order.
            /// </summary>
            /// This function starts with the first pattern and tries to match it
            /// against the <paramref name="input"/>. If it matches, it saves the
            /// index where it found the match, and and tries to match the next
            /// pattern starting at the index of the previous match.
            /// It continues until all the patterns have been matched.
            /// If a pattern didn't match, the function returns false.
            /// <param name="input">The input to match with.</param>
            /// <returns>True if all patterns match.</returns>
            public bool IsMatch(string input)
            {
                int matchIndex = 0;
                foreach (var r in _regexReplacersInOrder)
                {
                    var match = r.Regex.Match(input, matchIndex);
                    if (!match.Success)
                    {
                        return false;
                    }

                    matchIndex = match.Index;
                }
                return true;
            }

            /// <summary>
            /// Replaces text from the <paramref name="input"/> string with the replacer functions.
            /// </summary>
            /// This function starts with the first pattern and tries to match it
            /// against the <paramref name="input"/>. If it matches, it will call the replacer function
            /// associated with this pattern to replace the text in the input and save the index where
            /// the match occurred.
            /// It then tries to match the next pattern starting at the index of the previous match, and
            /// replace the text from the <paramref name="input"/> with the text from its associated
            /// replacer function.
            /// If the pattern doesn't have an associated replacer function, it simply saves the index
            /// of the match and move to the next pattern.
            /// If a pattern fails to match, an exception is thrown.
            /// <param name="input">The input string from where to replace the text.</param>
            /// <param name="propertyValue">The value of the <see cref="PropertyName"/> that will be passed
            /// to the replacer function.</param>
            /// <returns>The <paramref name="input"/> string with the text replaced in it.</returns>
            /// <exception cref="Exception">Throws an exception if any pattern fails to match.</exception>
            public string DoReplace(string input, string propertyValue)
            {
                int matchIndex = 0;
                foreach (var r in _regexReplacersInOrder)
                {
                    var match = r.Regex.Match(input, matchIndex);
                    if (!match.Success)
                    {
                        throw new Exception($"Could not find a match for {r}");
                    }
                    if (r.Replacer != null)
                    {
                        input = r.Regex.Replace(input, r.Replacer(propertyValue), count: 1, matchIndex);
                    }
                    matchIndex = match.Index;
                }

                return input;
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
            string newTagLine = $"{Environment.NewLine} - ";
            var tagsConfiguration = new StringBuilder();
            tagsConfiguration.Append("tags: ");
            tagsConfiguration.Append(newTagLine);
            tagsConfiguration.Append(string.Join(newTagLine, value.Split(',')));
            return tagsConfiguration.ToString();
        }

        public static string ReplaceProperties(string yaml, ISession session)
        {
            var replacers = new List<PropertyReplacer>
            {
                PropertyReplacer.For("APIKEY")
                    .Match("^[ #]*api_key:.*").ReplaceWith(value => $"api_key: {value}"),

                PropertyReplacer.For("SITE")
                    .Match("^[ #]*site:.*").ReplaceWith(value => $"site: {value}"),

                PropertyReplacer.For("HOSTNAME")
                    .Match("^[ #]*hostname:.*").ReplaceWith(value => $"hostname: {value}"),

                // When LOGS_ENABLED is specified, it's expected to uncomment logs_config as well
                // even if we don't modify the object.
                PropertyReplacer.For("LOGS_ENABLED")
                    .Match("^[ #]*logs_config:.*").ReplaceWith(_ => "logs_config:"),

                PropertyReplacer.For("LOGS_ENABLED")
                    .Match("^[ #]*logs_enabled:.*").ReplaceWith(value => $"logs_enabled: {value}"),

                PropertyReplacer.For("LOGS_DD_URL")
                    .Match("^[ #]*logs_config:.*").ReplaceWith(value => "logs_config:")
                    .Match("^[ #]*logs_dd_url:.*").ReplaceWith(value => $"  logs_dd_url: {value}"),

                PropertyReplacer.For("PROCESS_ENABLED")
                    .Match("^[ #]*process_config:").ReplaceWith(_ => "process_config:")
                    .Match("^[ #]*process_collection:").ReplaceWith(_ => "  process_collection:")
                    .Match("^[ #]*enabled:.*").ReplaceWith(value => $"    enabled: {value}"),

                // PROCESS_DD_URL does not replace but appends a new key to process_config
                PropertyReplacer.For("PROCESS_DD_URL")
                    .Match("^[ #]*process_config:").ReplaceWith(value => $"process_config:\n  process_dd_url: {value}"),

                PropertyReplacer.For("PROCESS_DISCOVERY_ENABLED")
                    .Match("^[ #]*process_config:").ReplaceWith(_ => "process_config:")
                    .Match("^[ #]*process_discovery:").ReplaceWith(_ => "  process_discovery:")
                    .Match("^[ #]*enabled:.*").ReplaceWith(value => $"    enabled: {value}"),

                PropertyReplacer.For("APM_ENABLED")
                    .Match("^[ #]*apm_config:").ReplaceWith(_ => "apm_config:")
                    .Match("^[ #]*enabled:.*").ReplaceWith(value => $"  enabled: {value}"),

                PropertyReplacer.For("TRACE_DD_URL")
                    .Match("^[ #]*apm_config:").ReplaceWith(_ => "apm_config:")
                    .Match("^[ #]*apm_dd_url:.*").ReplaceWith(value => $"  apm_dd_url: {value}"),

                // There are multiple cmd_port entry in the config, so we need
                // to match the top level one explicitly.
                PropertyReplacer.For("CMD_PORT")
                    .Match("^[# ]?[ ]*cmd_port:.*").ReplaceWith(value => $"cmd_port: {value}"),

                PropertyReplacer.For("DD_URL")
                    .Match("^[ #]*dd_url:.*").ReplaceWith(value => $"dd_url: {value}"),

                PropertyReplacer.For("PYVER")
                    .Match("^[ #]*python_version:.*").ReplaceWith(value => $"python_version: {value}"),

                PropertyReplacer.For("PROXY_HOST")
                    .Match("^[ #]*proxy:.*").ReplaceWith(value => FormatProxy(value, session)),

                PropertyReplacer.For("HOSTNAME_FQDN_ENABLED")
                    .Match("^[ #]*hostname_fqdn:.*").ReplaceWith(value => $"hostname_fqdn: {value}"),

                PropertyReplacer.For("TAGS")
                    .Match("^[ #]*tags:(?:(?:.|\n)*?)^[ #]*- <TAG_KEY>:<TAG_VALUE>").ReplaceWith(FormatTags),

                PropertyReplacer.For("EC2_USE_WINDOWS_PREFIX_DETECTION")
                    .Match("(^[ #]*ec2_use_windows_prefix_detection:.*|\\Z)").ReplaceWith(value => $"ec2_use_windows_prefix_detection: {value}")
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
                    yaml = replacer.DoReplace(yaml, session.Property(replacer.PropertyName));
                }
            }

            return yaml;
        }

        private static ActionResult WriteConfig(ISession session)
        {
            var configFolder = session.Property("APPLICATIONDATADIRECTORY");
            var datadogYaml = Path.Combine(configFolder, "datadog.yaml");
            var systemProbeYaml = Path.Combine(configFolder, "system-probe.yaml");
            var securityAgentYaml = Path.Combine(configFolder, "security-agent.yaml");
            var injectionControllerYaml = Path.Combine(configFolder, "apm-inject.yaml");
            try
            {
                if (!File.Exists(systemProbeYaml))
                {
                    File.Copy(systemProbeYaml + ".example", systemProbeYaml);
                }

                if (File.Exists(datadogYaml))
                {
                    session.Log($"{datadogYaml} exists, not modifying it.");
                    return ActionResult.Success;
                }
                string yaml;
                using (var input = new StreamReader(Path.Combine(configFolder, "datadog.yaml.example")))
                {
                    yaml = input.ReadToEnd();
                }

                yaml = ReplaceProperties(yaml, session);

                using (var output = new StreamWriter(datadogYaml))
                {
                    output.Write(yaml);
                }

                // Conditionally include the security agent YAML while it is in active development to make it easier
                // to build/ship without it.
                if (File.Exists(securityAgentYaml + ".example"))
                {
                    if (!File.Exists(securityAgentYaml))
                    {
                        File.Copy(securityAgentYaml + ".example", securityAgentYaml);
                    }
                }

                // Conditionally include the APM injection MSM while it is in active development to make it easier
                // to build/ship without it.
                if (File.Exists(injectionControllerYaml + ".example"))
                {
                    if (!File.Exists(injectionControllerYaml))
                    {
                        File.Copy(injectionControllerYaml + ".example", injectionControllerYaml);
                    }
                }
            }
            catch (Exception e)
            {
                session.Log($"User config could not be written: {e}");
                return ActionResult.Failure;
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
