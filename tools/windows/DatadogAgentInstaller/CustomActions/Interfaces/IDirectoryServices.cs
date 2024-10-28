using System.Security.AccessControl;

namespace Datadog.CustomActions.Interfaces
{
    public interface IDirectoryServices
    {
        bool Exists(string path);
        DirectorySecurity GetAccessControl(string path, AccessControlSections includeSections);
        DirectorySecurity GetAccessControl(string path);
        void SetAccessControl(string path, DirectorySecurity directorySecurity);
    }
}
