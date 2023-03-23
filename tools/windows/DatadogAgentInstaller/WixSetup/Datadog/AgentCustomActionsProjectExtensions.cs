using System.Linq;
using WixSharp;

namespace WixSetup.Datadog
{
    public static class AgentCustomActionsProjectExtensions
    {
        /// <summary>
        /// Helper method that will list all the properties deriving from <see cref="ManagedAction"/>
        /// in <see cref="AgentCustomActions"/> and add them to the <see cref="Project"/>.
        /// That way, no action is omitted by accident.
        /// </summary>
        /// <param name="project">The project to add the custom actions to.</param>
        /// <param name="agentCustomActions">The instance of the Agent custom action to inject.</param>
        /// <returns>The project, for chaining.</returns>
        public static Project SetCustomActions(this Project project, AgentCustomActions agentCustomActions)
        {
            project.Actions = project.Actions.Combine(
                agentCustomActions.GetType()
                    .GetProperties()
                    .Where(p => p.PropertyType.IsAssignableFrom(typeof(ManagedAction)))
                    .Select(p => (Action)p.GetValue(agentCustomActions, null))
                    .ToArray()
            );
            return project;
        }

        /// <summary>
        /// Adds WiX User elements to the <see cref="Project"/> for the Agent user account.
        /// </summary>
        ///
        /// <remarks>
        /// The User Element handles creation of the user account, and setting of the password and USER_INFO flags
        /// such as UF_DONT_EXPIRE_PASSWD.
        /// See <a href="https://github.com/wixtoolset/wix3/blob/c02e48ec301a60eba88a3b519d47e88eeaa4c978/src/ext/ca/serverca/scaexec/scaexec.cpp#L1722-L1725">WIX CreateUser custom action</a> and <a href="https://wixtoolset.org/docs/v3/xsd/util/user/">WiX User Element</a>.
        /// Other account settings such as group membership and user rights assignment are handled in our own custom actions
        /// and are not affected by this element.
        ///
        /// Two User elements are used because WiX does not allow YesNoType to be configurable with Properties.
        /// Only one is ever enabled at a time and are selected by our custom actions.
        /// </remarks>
        /// <param name="project">The project to add the WiX User elements to.</param>
        /// <returns>The project, for chaining.</returns>
        public static Project AddAgentUser(this Project project)
        {
            project.GenericItems = project.GenericItems.Combine(
                // This User element is used when the user account is a regular account and the DDAGENTUSER_PASSWORD field is blank.
                // The installer will create the account if needed and also change an existing account's password and USER_FLAGS.
                //
                // Use case: The customer expects the installer to manage its own user account on the local machine.
                new User(new Id("InstallerManagedAgentUser"), "")
                {
                    // CreateUser fails with ERROR_BAD_USERNAME if Name is a fully qualified user name
                    Name = "[DDAGENTUSER_PROCESSED_NAME]",
                    Domain = "[DDAGENTUSER_PROCESSED_DOMAIN]",
                    Password = "[DDAGENTUSER_PROCESSED_PASSWORD]",
                    PasswordNeverExpires = true,
                    RemoveOnUninstall = false,
                    FailIfExists = false,
                    CreateUser = true,
                    UpdateIfExists = true,
                },
                // This User element is used when a regular account with a DDAGENTUSER_PASSWORD or a service account is provided.
                // Regular accounts will be created if they do not exist and the USER_INFO flags will be set.
                // If the account already exists then it will not be modified.
                //
                // Use case: The customer manages and provides the user account for the Agent to use.
                new User(new Id("UserManagedAgentUser"), "")
                {
                    // CreateUser fails with ERROR_BAD_USERNAME if Name is a fully qualified user name
                    Name = "[DDAGENTUSER_PROCESSED_NAME]",
                    Domain = "[DDAGENTUSER_PROCESSED_DOMAIN]",
                    Password = "[DDAGENTUSER_PROCESSED_PASSWORD]",
                    PasswordNeverExpires = true,
                    RemoveOnUninstall = false,
                    FailIfExists = false,
                    CreateUser = true,
                    UpdateIfExists = false,
                }
            );
            return project;
        }
    }
}
