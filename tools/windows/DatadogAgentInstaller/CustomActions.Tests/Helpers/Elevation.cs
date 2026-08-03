using System;
using System.Security.Principal;
using Xunit;

namespace CustomActions.Tests.Helpers
{
    /// <summary>
    /// Whether the tests that need an elevated process can run, and why not when they cannot.
    /// </summary>
    /// <remarks>
    /// Skipped only for a developer running them from an unelevated shell, never in the CI, so a
    /// runner that cannot run them fails instead of quietly skipping. Same as
    /// skipIfDontHavePrivileges in pkg/fleet/installer/paths/paths_windows_test.go.
    /// </remarks>
    internal static class Elevation
    {
        /// <summary>
        /// Whether the elevated tests are going to run, for setup that itself needs the privileges.
        /// </summary>
        internal static bool IsAvailable => SkipReason() == null;

        internal static string SkipReason()
        {
            if (IsCi())
            {
                return null;
            }

            return new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(WindowsBuiltInRole.Administrator)
                ? null
                : "requires an elevated process";
        }

        private static bool IsCi()
        {
            return !string.IsNullOrEmpty(Environment.GetEnvironmentVariable("CI")) ||
                   !string.IsNullOrEmpty(Environment.GetEnvironmentVariable("CI_JOB_ID"));
        }
    }

    /// <summary>
    /// A <see cref="FactAttribute"/> that is skipped when the process is not elevated,
    /// see <see cref="Elevation"/>.
    /// </summary>
    public sealed class ElevatedFactAttribute : FactAttribute
    {
        public ElevatedFactAttribute()
        {
            Skip = Elevation.SkipReason();
        }
    }

    /// <summary>
    /// A <see cref="TheoryAttribute"/> that is skipped when the process is not elevated,
    /// see <see cref="Elevation"/>.
    /// </summary>
    public sealed class ElevatedTheoryAttribute : TheoryAttribute
    {
        public ElevatedTheoryAttribute()
        {
            Skip = Elevation.SkipReason();
        }
    }
}
