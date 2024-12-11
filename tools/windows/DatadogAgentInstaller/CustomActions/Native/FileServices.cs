using System.IO;
using System.Security.AccessControl;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Native
{
    class FileServices : IFileServices
    {
        public bool Exists(string path)
        {
            return File.Exists(path);
        }

        public FileSecurity GetAccessControl(string path, AccessControlSections includeSections)
        {
            return File.GetAccessControl(path, includeSections);
        }

        public FileSecurity GetAccessControl(string path)
        {
            return File.GetAccessControl(path);
        }

        public void SetAccessControl(string path, FileSecurity fileSecurity)
        {
            File.SetAccessControl(path, fileSecurity);
        }
    }
}
