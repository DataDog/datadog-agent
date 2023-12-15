using System.Collections.Generic;
using System.Linq;
using WixSharp;

namespace WixSetup
{
    public class CustomAction<T> : ManagedAction
    {
        public CustomAction(
            Id id,
            CustomActionMethod method)
            : base(id, method, typeof(T).Assembly.Location)
        {
            RefAssemblies = typeof(T).GetReferencesAssembliesPaths().ToArray();
        }

        public CustomAction(
            CustomActionMethod method)
            : base(method, typeof(T).Assembly.Location)
        {
            RefAssemblies = typeof(T).GetReferencesAssembliesPaths().ToArray();
        }

        public CustomAction(
            CustomActionMethod action,
            Return returnType,
            When when,
            Step step)
            : base(action, typeof(T).Assembly.Location, returnType, when, step, Condition.Always)
        {
            RefAssemblies = typeof(T).GetReferencesAssembliesPaths().ToArray();
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
            RefAssemblies = typeof(T).GetReferencesAssembliesPaths().ToArray();
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
            RefAssemblies = typeof(T).GetReferencesAssembliesPaths().ToArray();
        }

        public CustomAction(
            Id id,
            CustomActionMethod action,
            Return returnType,
            When when,
            Step step)
            : base(id, action, typeof(T).Assembly.Location, returnType, when, step, Condition.Always)
        {
            RefAssemblies = typeof(T).GetReferencesAssembliesPaths().ToArray();
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
            RefAssemblies = typeof(T).GetReferencesAssembliesPaths().ToArray();
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
