using System;
using System.Security.AccessControl;
using Datadog.CustomActions.Interfaces;
using Newtonsoft.Json;

namespace Datadog.CustomActions.Rollback
{
    class FileStorageRollbackData : IRollbackAction
    {
        [JsonProperty("FilePath")] private string _filePath;
        [JsonProperty("Content")] private string _content;

        [JsonConstructor]
        public FileStorageRollbackData()
        {
        }

        public FileStorageRollbackData(string filePath)
        {
            _filePath = filePath;

            // check that file path exists
            if (!System.IO.File.Exists(filePath))
            {
                _content = null;
            }
            else
            {
                // read the content and base64 encode it
                var fileContent = System.IO.File.ReadAllBytes(filePath);
                _content = Convert.ToBase64String(fileContent);
            }

        }

        /// <summary>
        /// Write fileContent to the @FilePath for restore purposes.
        /// </summary>
        /// <remarks>
        /// Files with no content will not be created.
        /// </remarks>
        public void Restore(ISession session, IFileSystemServices _, IServiceController __)
        {
            // restore the file content
            if (_content == null)
            {
                session.Log($"File {_filePath} has no content");
                return;
            }
            var fileContent = Convert.FromBase64String(_content);
            System.IO.File.WriteAllBytes(_filePath, fileContent);
            session.Log($"Restored file {_filePath}");
        }
    }
}
