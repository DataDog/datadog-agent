using System.IO;
using Microsoft.Deployment.WindowsInstaller;
using YamlDotNet.Serialization.NamingConventions;
using YamlDotNet.Serialization;

namespace Datadog.CustomActions
{
    public class ConfigUserActions
    {
        class DatadogConfig
        {
            public string ApiKey { get; set; }
            public string Site { get; set; }
        }

        [CustomAction]
        public static ActionResult ReadConfig(Session session)
        {
            var configFolder = session["APPLICATIONDATADIRECTORY"];

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
            catch
            {
                // ignored
            }


            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult WriteConfig(Session session)
        {



            return ActionResult.Success;
        }
    }
}
