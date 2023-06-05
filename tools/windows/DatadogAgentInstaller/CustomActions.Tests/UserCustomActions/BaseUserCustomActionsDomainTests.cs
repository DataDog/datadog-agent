namespace CustomActions.Tests.UserCustomActions
{
    public class BaseUserCustomActionsDomainTests
    {
        public UserCustomActionsTestSetup Test { get; } = new();

        public string Domain => Test.NativeMethods.Object.GetComputerDomain();
    }
}
