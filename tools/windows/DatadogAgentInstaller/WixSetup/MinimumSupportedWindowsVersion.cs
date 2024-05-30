using System.Xml.Linq;
using WixSharp;

namespace WixSetup.Datadog_Agent
{
    /// <summary>
    /// Adds a LaunchCondition in the MSI to control the oldest OS supported.
    /// </summary>
    public class MinimumSupportedWindowsVersion : WixEntity, IGenericEntity
    {
        private static readonly Condition Client = Condition.Create("MsiNTProductType = 1");
        private static readonly Condition Server = Condition.Create("MsiNTProductType >= 2");

        private readonly Condition _condition;

        private MinimumSupportedWindowsVersion(
            string name,
            Condition condition)
        {
            Name = name;
            _condition = condition;
        }

        private MinimumSupportedWindowsVersion(
            string name,
            int versionNt,
            int windowsBuild,
            Condition clientOrServer)
        {
            Name = name;
            _condition = Condition.Create($"(VersionNT64 >= {versionNt} AND DDAGENT_WINDOWSBUILD >= {windowsBuild})") &
                         clientOrServer;
        }

        public static MinimumSupportedWindowsVersion operator |(
            MinimumSupportedWindowsVersion a,
            MinimumSupportedWindowsVersion b)
        {
            return new MinimumSupportedWindowsVersion($"{a.Name} or {b.Name}", a._condition | b._condition);
        }

        public static MinimumSupportedWindowsVersion WindowsServer2012 = new("Windows Server 2012", 602, 9200, Server);
        public static MinimumSupportedWindowsVersion WindowsServer2012R2 = new("Windows Server 2012 R2", 603, 9600, Server);

        // To maintain compatibility, the VersionNT value is 603 for Windows 10, Windows Server 2016, and Windows Server 2019.
        // https://learn.microsoft.com/en-US/troubleshoot/windows-client/application-management/versionnt-value-for-windows-10-server
        public static MinimumSupportedWindowsVersion WindowsServer2016 = new("Windows Server 2016", 603, 9800, Server);
        public static MinimumSupportedWindowsVersion WindowsServer2019 = new("Windows Server 2019", 603, 17623, Server);
        public static MinimumSupportedWindowsVersion WindowsServer2022 = new("Windows Server 2022", 603, 20348, Server);

        public static MinimumSupportedWindowsVersion Windows8 = new("Windows 8", 602, 9200, Client);
        public static MinimumSupportedWindowsVersion Windows8_1 = new("Windows 8.1", 603, 9600, Client);
        // Windows 10 RTM (1507) shipped with 10240, so it's safe to assume anything above 10000 is Windows 10.
        public static MinimumSupportedWindowsVersion Windows10 = new("Windows 10", 603, 10000, Client);
        // Official first publicly available preview of Windows 11 bore the build number 22000.51, so assume anything
        // above 22000 is Windows 11.
        public static MinimumSupportedWindowsVersion Windows11 = new("Windows 11", 603, 22000, Client);

        public void Process(ProcessingContext context)
        {
            var elem = new XElement("Condition");
            elem.Add(_condition.ToCData());
            elem.SetAttributeValue("Message", $"This application is only supported on {Name}, and later.");
            context.XParent.AddElement(elem);
        }
    }
}
