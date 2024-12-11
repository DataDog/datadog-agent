using System.IO;
using System.Security.AccessControl;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Native
{
    class DirectoryServices : IDirectoryServices
    {
        public bool Exists(string path)
        {
            return Directory.Exists(path);
        }

        public DirectorySecurity GetAccessControl(string path, AccessControlSections includeSections)
        {
            return Directory.GetAccessControl(path, includeSections);
        }

        public DirectorySecurity GetAccessControl(string path)
        {
            return Directory.GetAccessControl(path);
        }

        public void SetAccessControl(string path, DirectorySecurity directorySecurity)
        {
            Directory.SetAccessControl(path, directorySecurity);
        }
    }
}
