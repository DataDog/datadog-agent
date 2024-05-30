using System.Linq;
using WixSharp;

namespace WixSetup.Datadog_Agent
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
        public static Project SetCustomActions(this Project project, object agentCustomActions)
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
    }
}
