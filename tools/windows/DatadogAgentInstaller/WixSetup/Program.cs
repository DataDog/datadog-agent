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
            installer.ConfigureProject().BuildMsi();
        }
    }

}
