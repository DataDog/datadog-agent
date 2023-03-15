using WixSharp;

namespace WixSetup.Datadog
{
    /// <summary>
    /// Adds a LaunchCondition in the MSI to control the oldest OS supported.
    /// </summary>
    public class SupportedWindowsVersion : WixEntity, IGenericEntity
    {
        private SupportedWindowsVersion(string name, int versionNt, int windowsBuild)
        {
            Name = name;
            VersionNT = versionNt;
            WindowsBuild = windowsBuild;
        }

        // ReSharper disable once InconsistentNaming
        public int VersionNT { get; }
        public int WindowsBuild { get; }

        public static SupportedWindowsVersion WindowsServer2012R2 = new("Windows Server 2012 R2", 603,  9600 );
        public static SupportedWindowsVersion WindowsServer2016   = new("Windows Server 2016",    1000, 9800 );
        public static SupportedWindowsVersion WindowsServer2019   = new("Windows Server 2019",    1000, 17623);
        public static SupportedWindowsVersion WindowsServer2022   = new("Windows Server 2022",    1000, 20348);

        public void Process(ProcessingContext context)
        {
            context.XParent.AddElement(
                "Condition",
                $"Message=This application is only supported on {Name}, or higher.",
                $"Installed OR (VersionNT64 >= {VersionNT} AND WindowsBuild >= {WindowsBuild})>"
            );
        }
    }
}
