using System.Text;
using System.Runtime.InteropServices;

public static class NativeMethods
{
    [DllImport("credui.dll", EntryPoint = "CredUIParseUserNameW", CharSet = CharSet.Unicode)]
    public static extern CredUIReturnCodes CredUIParseUserName(
        string userName,
        StringBuilder user,
        int userMaxChars,
        StringBuilder domain,
        int domainMaxChars);
}
