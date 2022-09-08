using System;
using System.IO;
using System.Windows.Forms;
using Microsoft.Deployment.WindowsInstaller;
using YamlDotNet.RepresentationModel;

namespace CustomActions
{
    public class ConfigUserActions
    {
        private const string Document = @"---

#########################
## Basic Configuration ##
#########################

## @param api_key - string - required
## @env DD_API_KEY - string - required
## The Datadog API key to associate your Agent's data with your organization.
## Create a new API key here: https://app.datadoghq.com/account/settings
#
api_key: coolapikeydog

## @param site - string - optional - default: datadoghq.com
## @env DD_SITE - string - optional - default: datadoghq.com
## The site of the Datadog intake to send Agent data to.
## Set to 'datadoghq.eu' to send data to the EU site.
## Set to 'us3.datadoghq.com' to send data to the US3 site.
## Set to 'ddog-gov.com' to send data to the US1-FED site.
#
site: datadoghq.eu
";

        [CustomAction]
        public static ActionResult MyAction(Session session)
        {
            // Setup the input
            var input = new StringReader(Document);

            // Load the stream
            var yaml = new YamlStream();
            yaml.Load(input);

            // Examine the stream
            var mapping =
                (YamlMappingNode)yaml.Documents[0].RootNode;

            foreach (var entry in mapping.Children)
            {
                Console.WriteLine(((YamlScalarNode)entry.Key).Value);
            }

            MessageBox.Show("API Key: " + ((YamlScalarNode)mapping.Children[0].Value).Value + " (CLR: v" + Environment.Version + ")", "Embedded Managed CA (" + (Is64BitProcess ? "x64" : "x86") + ")");
            session.Log("Begin MyAction Hello World");

            return ActionResult.Success;
        }

        public static bool Is64BitProcess => IntPtr.Size == 8;
    }
}