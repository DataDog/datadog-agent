using Datadog.CustomActions;
using WixSharp;

namespace WixSetup.Datadog
{
    public class AgentCustomActions
    {
        public ManagedAction ReadConfig { get; }

        public ManagedAction WriteConfig { get; }

        public ManagedAction ProcessDdAgentUserCredentials { get; }


        public ManagedAction DecompressPythonDistributions { get; }

        public AgentCustomActions()
        {
            // We need to explicitly set the ID since that we are going to reference before the Build* call.
            // See <see cref="WixSharp.WixEntity.Id" /> for more information.
            ReadConfig = new CustomAction<ConfigCustomActions>(
                    new Id("ReadConfigCustomAction"),
                    ConfigCustomActions.ReadConfig
                )
                .SetProperties("APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY]");

            WriteConfig = new CustomAction<ConfigCustomActions>(
                    new Id("WriteConfigCustomAction"),
                    ConfigCustomActions.WriteConfig
                )
                .SetProperties(
                    "APPLICATIONDATADIRECTORY=[APPLICATIONDATADIRECTORY], " +
                    "PROJECTLOCATION=[PROJECTLOCATION], " +
                    "SYSPROBE_PRESENT=[SYSPROBE_PRESENT], " +
                    "ADDLOCAL=[ADDLOCAL], " +
                    "APIKEY=[APIKEY], " +
                    "TAGS=[TAGS], " +
                    "HOSTNAME=[HOSTNAME], " +
                    "PROXY_HOST=[PROXY_HOST], " +
                    "PROXY_PORT=[PROXY_PORT], " +
                    "PROXY_USER=[PROXY_USER], " +
                    "PROXY_PASSWORD=[PROXY_PASSWORD], " +
                    "LOGS_ENABLED=[LOGS_ENABLED], " +
                    "APM_ENABLED=[APM_ENABLED], " +
                    "PROCESS_ENABLED=[PROCESS_ENABLED], " +
                    "PROCESS_DISCOVERY_ENABLED=[PROCESS_DISCOVERY_ENABLED], " +
                    "CMD_PORT=[CMD_PORT], " +
                    "SITE=[SITE], " +
                    "DD_URL=[DD_URL], " +
                    "LOGS_DD_URL=[LOGS_DD_URL], " +
                    "PROCESS_DD_URL=[PROCESS_DD_URL], " +
                    "TRACE_DD_URL=[TRACE_DD_URL], " +
                    "PYVER=[PYVER], " +
                    "HOSTNAME_FQDN_ENABLED=[HOSTNAME_FQDN_ENABLED], " +
                    "NPM=[NPM], " +
                    "EC2_USE_WINDOWS_PREFIX_DETECTION=[EC2_USE_WINDOWS_PREFIX_DETECTION], " +
                    "OVERRIDE_INSTALLATION_METHOD=[OVERRIDE_INSTALLATION_METHOD]");

            ProcessDdAgentUserCredentials = new CustomAction<UserCustomActions>(
                    new Id("ProcessDdAgentUserCredentials"),
                    UserCustomActions.ProcessDdAgentUserCredentials,
                    Return.check,
                    When.Before,
                    Step.InstallInitialize,
                    Condition.NOT_Installed & Condition.NOT_BeingRemoved
                )
                .SetProperties("DDAGENTUSER_NAME=[DDAGENTUSER_NAME], DDAGENTUSER_PASSWORD=[DDAGENTUSER_PASSWORD]")
                .HideTarget(true);

            DecompressPythonDistributions = new CustomAction<DecompressCustomActions>(
                new Id("DecompressPythonDistributions"),
                DecompressCustomActions.DecompressPythonDistributions,
                Return.check,
                When.After,
                Step.InstallFinalize,
                Condition.NOT_Installed & Condition.NOT_BeingRemoved
                )
                .SetProperties("PROJECTLOCATION=[PROJECTLOCATION]");
        }
    }
}
