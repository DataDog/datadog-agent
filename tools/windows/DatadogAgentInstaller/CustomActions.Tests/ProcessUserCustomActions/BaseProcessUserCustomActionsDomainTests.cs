namespace CustomActions.Tests.ProcessUserCustomActions
{
    public class BaseProcessUserCustomActionsDomainTests
    {
        public ProcessUserCustomActionsTestSetup Test { get; } = new();

        public string Domain => Test.NativeMethods.Object.GetComputerDomain();
    }
}
