using WixSharp;
using WixSetup.Datadog;

namespace WixSetup
{
    internal class Program
    {
        private static void Main()
        {
            var installer = new AgentInstaller();
            Compiler.LightOptions += "-sval -reusecab -cc \"cabcache\"";
            // ServiceConfig functionality is documented in the Windows Installer SDK to "not [work] as expected." Consider replacing ServiceConfig with the WixUtilExtension ServiceConfig element.
            Compiler.CandleOptions += "-sw1150";
            installer.ConfigureProject().BuildMsi();
        }
    }

}
