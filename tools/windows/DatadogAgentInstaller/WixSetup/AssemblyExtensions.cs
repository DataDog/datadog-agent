using System;
using System.Collections.Generic;
using System.Reflection;

namespace WixSetup
{
    public static class AssemblyExtensions
    {
        public static IEnumerable<string> GetReferencesAssembliesPaths(this Type type)
        {
            yield return type.Assembly.Location;

            foreach (var assemblyName in type.Assembly.GetReferencedAssemblies())
            {
                yield return Assembly.ReflectionOnlyLoad(assemblyName.FullName).Location;
            }
        }
    }
}
