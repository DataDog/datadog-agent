using Microsoft.Deployment.WindowsInstaller;
using System;
using System.Collections.Generic;
using System.Windows.Forms;
using YamlDotNet.Core;
using YamlDotNet.Serialization.NamingConventions;
using YamlDotNet.Serialization;
using System.IO;
using YamlDotNet.RepresentationModel;
using System.Text;

public class CustomActions
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

    /// <summary>
    /// Extracts the domain and user account name from a fully qualified user name.
    /// </summary>
    /// <param name="userName">A <see cref="string"/> that contains the user name to be parsed. The name must be in UPN or down-level format, or a certificate.</param>
    /// <param name="user">A <see cref="string"/> that receives the user account name.</param>
    /// <param name="domain">A <see cref="string"/> that receives the domain name. If <paramref name="userName"/> specifies a certificate, pszDomain will be <see langword="null"/>.</param>
    /// <returns>
    ///     <see langword="true"/> if the <paramref name="userName"/> contains a domain and a user-name; otherwise, <see langword="false"/>.
    /// </returns>
    [System.Diagnostics.CodeAnalysis.SuppressMessage("Microsoft.Design", "CA1021:AvoidOutParameters")]
    public static bool ParseUserName(string userName, out string user, out string domain)
    {
        if (string.IsNullOrEmpty(userName))
            throw new ArgumentNullException("userName");

        StringBuilder userBuilder = new StringBuilder();
        StringBuilder domainBuilder = new StringBuilder();

        CredUIReturnCodes returnCode = NativeMethods.CredUIParseUserName(userName, userBuilder, int.MaxValue, domainBuilder, int.MaxValue);
        switch (returnCode)
        {
            case CredUIReturnCodes.NO_ERROR: // The username is valid.
                user = userBuilder.ToString();
                domain = domainBuilder.ToString();
                return true;

            case CredUIReturnCodes.ERROR_INVALID_ACCOUNT_NAME: // The username is not valid.
                user = userName;
                domain = null;
                return false;

            // Impossible to reach this state
            //case CredUIReturnCodes.ERROR_INSUFFICIENT_BUFFER: // One of the buffers is too small.
            //    throw new OutOfMemoryException();

            case CredUIReturnCodes.ERROR_INVALID_PARAMETER: // ulUserMaxChars or ulDomainMaxChars is zero OR userName, user, or domain is NULL.
                throw new ArgumentNullException("userName");

            default:
                user = null;
                domain = null;
                return false;
        }
    }

    [CustomAction]
    public static ActionResult CreateUser(Session session)
    {
        string userName, domain;
        ParseUserName(session["DDAGENTUSER_NAME"], out userName, out domain);
        MessageBox.Show(domain + @"\" + userName);
        return ActionResult.Success;
    }

    [CustomAction]
    public static ActionResult CreateServices(Session session)
    {


        return ActionResult.Success;
    }

    public static bool Is64BitProcess
    {
        get { return IntPtr.Size == 8; }
    }
}
