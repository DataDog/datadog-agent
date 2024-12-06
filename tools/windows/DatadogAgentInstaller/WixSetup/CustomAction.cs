using System.Collections.Generic;
using System.Linq;
using WixSharp;

namespace WixSetup
{
    public class CustomAction<T> : ManagedAction
    {
        private string[] GetRefAssemblies()
        {
            // Include references of assembly where the custom action entrypoint is defined
            IEnumerable<string> refs = typeof(T).GetReferencesAssembliesPaths();
            // Include references of common CustomActions assembly
            refs = refs.Concat(
                typeof(Datadog.CustomActions.Constants).GetReferencesAssembliesPaths().ToArray()
            );
            return refs.ToArray();
        }
        public CustomAction(
            Id id,
            CustomActionMethod method)
            : base(id, method, typeof(T).Assembly.Location)
        {
            RefAssemblies = GetRefAssemblies();
        }

        public CustomAction(
            CustomActionMethod method)
            : base(method, typeof(T).Assembly.Location)
        {
            RefAssemblies = GetRefAssemblies();
        }

        public CustomAction(
            CustomActionMethod action,
            Return returnType,
            When when,
            Step step)
            : base(action, typeof(T).Assembly.Location, returnType, when, step, Condition.Always)
        {
            RefAssemblies = GetRefAssemblies();
        }

        public CustomAction(
            Id id,
            CustomActionMethod action,
            Return returnType,
            When when,
            Step step,
            Condition condition)
            : base(id, action, typeof(T).Assembly.Location, returnType, when, step, condition)
        {
            RefAssemblies = GetRefAssemblies();
        }

        public CustomAction(
            Id id,
            CustomActionMethod action,
            Return returnType,
            When when,
            Step step,
            Condition condition,
            Sequence sequence)
            : base(id, action, typeof(T).Assembly.Location, returnType, when, step, condition, sequence)
        {
            RefAssemblies = GetRefAssemblies();
        }

        public CustomAction(
            Id id,
            CustomActionMethod action,
            Return returnType,
            When when,
            Step step)
            : base(id, action, typeof(T).Assembly.Location, returnType, when, step, Condition.Always)
        {
            RefAssemblies = GetRefAssemblies();
        }

        public CustomAction(
            CustomActionMethod action,
            Return returnType,
            When when,
            Step step,
            Condition condition,
            CustomActionMethod rollback)
            : base(action, typeof(T).Assembly.Location, returnType, when, step, condition, rollback)
        {
            RefAssemblies = GetRefAssemblies();
        }

        public CustomAction<T> HideTarget(bool hideTarget)
        {
            if (Attributes == null)
            {
                Attributes = new Dictionary<string, string>();
            }

            Attributes["HideTarget"] = hideTarget ? "yes" : "no";
            return this;
        }

        public CustomAction<T> SetProperties(string properties)
        {
            UsesProperties = properties;
            return this;
        }
    }
}
