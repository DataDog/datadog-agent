using System;
using System.Collections.Generic;
using System.Reflection;

namespace WixSetup
{
    public static class AssemblyExtensions
    {
        /// <summary>
        /// .NET Framework BCL assembly names that are always present on the target system.
        /// These must not be embedded in the CA package; the CLR loads them from the GAC/Framework.
        /// </summary>
        private static readonly HashSet<string> FrameworkAssemblyNames = new(StringComparer.OrdinalIgnoreCase)
        {
            "mscorlib",
            "System",
            "System.Core",
            "System.Xml",
            "System.Xml.Linq",
            "Microsoft.CSharp",
            "System.Configuration.Install",
            "System.DirectoryServices",
            "System.Drawing",
            "System.ServiceProcess",
            "System.Windows.Forms",
        };

        public static IEnumerable<string> GetReferencesAssembliesPaths(this Type type)
        {
            yield return type.Assembly.Location;

            foreach (var assemblyName in type.Assembly.GetReferencedAssemblies())
            {
                if (FrameworkAssemblyNames.Contains(assemblyName.Name))
                {
                    continue;
                }

                yield return Assembly.ReflectionOnlyLoad(assemblyName.FullName).Location;
            }
        }
    }
}
