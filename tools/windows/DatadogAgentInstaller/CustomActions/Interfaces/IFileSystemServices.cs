using System.IO;
using System.Security.AccessControl;

namespace Datadog.CustomActions.Interfaces
{
    public interface IFileSystemServices
    {
        bool Exists(string path);
        bool IsFile(string path);
        bool IsDirectory(string path);
        FileSystemSecurity GetAccessControl(string path, AccessControlSections includeSections);
        FileSystemSecurity GetAccessControl(string path);
        void SetAccessControl(string path, FileSystemSecurity fileSecurity);
    }
}
