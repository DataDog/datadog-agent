using System.Security.AccessControl;

namespace Datadog.CustomActions.Interfaces
{
    public interface IFileServices
    {
        bool Exists(string path);
        FileSecurity GetAccessControl(string path, AccessControlSections includeSections);
        FileSecurity GetAccessControl(string path);
        void SetAccessControl(string path, FileSecurity fileSecurity);
    }
}
