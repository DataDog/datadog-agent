using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Linq;
using System.Security.AccessControl;
using System.Text;
using System.Threading.Tasks;
using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Native
{
    class FileSystemServices : IFileSystemServices
    {
        private readonly IDirectoryServices _directoryServices;
        private readonly IFileServices _fileServices;

        public FileSystemServices(
            IDirectoryServices ds,
            IFileServices fs)
        {
            _directoryServices = ds;
            _fileServices = fs;
        }

        public FileSystemServices()
            : this(
                new DirectoryServices(),
                new FileServices())
        {
        }

        public bool Exists(string path)
        {
            return _directoryServices.Exists(path) || _fileServices.Exists(path);
        }

        public bool IsDirectory(string path)
        {
            return _directoryServices.Exists(path);
        }

        public bool IsFile(string path)
        {
            return _fileServices.Exists(path);
        }

        public FileSystemSecurity GetAccessControl(string path, AccessControlSections includeSections)
        {
            if (IsDirectory(path))
            {
                return _directoryServices.GetAccessControl(path, includeSections);
            }
            else if (IsFile(path))
            {
                return _fileServices.GetAccessControl(path, includeSections);
            }
            else
            {
                throw new Exception($"{path} is not a file or directory");
            }
        }

        public FileSystemSecurity GetAccessControl(string path)
        {
            if (IsDirectory(path))
            {
                return _directoryServices.GetAccessControl(path);
            }
            else if (IsFile(path))
            {
                return _fileServices.GetAccessControl(path);
            }
            else
            {
                throw new Exception($"{path} is not a file or directory");
            }
        }

        public void SetAccessControl(string path, FileSystemSecurity fileSecurity)
        {
            if (IsDirectory(path))
            {
                _directoryServices.SetAccessControl(path, (DirectorySecurity)fileSecurity);
            }
            else if (IsFile(path))
            {
                _fileServices.SetAccessControl(path, (FileSecurity)fileSecurity);
            }
            else
            {
                throw new Exception($"{path} is not a file or directory");
            }
        }
    }
}
