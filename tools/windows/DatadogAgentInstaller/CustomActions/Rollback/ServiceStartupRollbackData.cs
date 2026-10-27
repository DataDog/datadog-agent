using System.Collections.Generic;
using Datadog.CustomActions.Interfaces;
using Newtonsoft.Json;

namespace Datadog.CustomActions.Rollback
{
    internal sealed class ServiceStartupRollbackData : IRollbackAction
    {
        [JsonProperty("Settings")]
        private List<ServiceCustomAction.StartupSettings> _settings;

        [JsonConstructor]
        public ServiceStartupRollbackData()
        {
        }

        public ServiceStartupRollbackData(List<ServiceCustomAction.StartupSettings> settings)
        {
            _settings = settings;
        }

        public void Restore(ISession session, IFileSystemServices _, IServiceController serviceController)
        {
            ServiceCustomAction.RestoreServiceStartup(session, serviceController, _settings);
        }
    }
}
