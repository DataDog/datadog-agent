using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.ServiceProcess;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Newtonsoft.Json;
using WixToolset.Dtf.WindowsInstaller;

namespace Datadog.CustomActions
{
    // The outer MSI owns this snapshot. An uninstall invoked by RemoveExistingProducts
    // must not replace it with the temporarily disabled settings.
    public class ServiceStartupCustomAction
    {
        internal sealed class StartupSettings
        {
            public string Name { get; set; }
            public ServiceStartMode StartType { get; set; }
            public bool DelayedAutoStart { get; set; }
        }

        internal static readonly string[] ProtectedServices =
        {
            Constants.AgentServiceName,
            Constants.ProcmgrServiceName,
            Constants.InstallerServiceName
        };

        internal static void Disable(IServiceController controller, Action<List<StartupSettings>> save, Action stop)
        {
            var services = controller.Services;
            var settings = new List<StartupSettings>();
            foreach (var name in ProtectedServices)
            {
                var service = services.FirstOrDefault(s => s.ServiceName.Equals(name, StringComparison.OrdinalIgnoreCase));
                if (service == null)
                {
                    continue;
                }
                settings.Add(new StartupSettings
                {
                    Name = name,
                    StartType = service.StartType,
                    DelayedAutoStart = service.DelayedAutoStart
                });
            }

            // Persist the entire snapshot before the first mutation. A failed disable or
            // stop is undone by the registered rollback action, including partial failures.
            save(settings);
            foreach (var setting in settings)
            {
                controller.SetStartType(setting.Name, ServiceStartMode.Disabled);
            }
            stop();
        }

        internal static bool Restore(ISession session, IServiceController controller, IEnumerable<StartupSettings> settings)
        {
            var restored = true;
            // Enable the core Agent last, after its dependent services are usable again.
            foreach (var setting in settings.Reverse())
            {
                try
                {
                    if (!controller.ServiceExists(setting.Name))
                    {
                        session.Log($"Service {setting.Name} no longer exists, skipping startup settings restore");
                        continue;
                    }
                    controller.SetDelayedAutoStart(setting.Name, setting.DelayedAutoStart);
                    controller.SetStartType(setting.Name, setting.StartType);
                }
                catch (Exception e)
                {
                    // One failed restore must not prevent restoring the other services.
                    restored = false;
                    session.Log($"Failed to restore startup settings for {setting.Name}: {e}");
                }
            }
            // Deliberately do not start services on rollback.
            return restored;
        }

        private static string SnapshotPath(ISession session)
        {
            // Generated per transaction, not per package: a failed save cannot load stale
            // settings from an earlier repair/upgrade. Parsing also constrains the filename.
            var id = Guid.Parse(session.Property("DD_SERVICE_STARTUP_ID"));
            var directory = Path.Combine(Path.GetTempPath(), "datadog-installer", "service-startup");
            SecureDirectory.CreateAndSecure(session, Path.GetDirectoryName(directory));
            SecureDirectory.CreateAndSecure(session, directory);
            return Path.Combine(directory, id.ToString("N") + ".json");
        }

        internal static void SaveSnapshot(ISession session, List<StartupSettings> settings)
        {
            File.WriteAllText(SnapshotPath(session), JsonConvert.SerializeObject(settings));
        }

        public static ActionResult Restore(Session session)
        {
            var wrapper = new SessionWrapper(session);
            try
            {
                var path = SnapshotPath(wrapper);
                if (File.Exists(path))
                {
                    var settings = JsonConvert.DeserializeObject<List<StartupSettings>>(File.ReadAllText(path));
                    if (Restore(wrapper, new Native.ServiceController(), settings))
                    {
                        File.Delete(path);
                    }
                }
            }
            catch (Exception e)
            {
                session.Log($"Failed to restore service startup settings: {e}");
            }
            return ActionResult.Success;
        }

        public static ActionResult Enable(Session session)
        {
            return Enable(new SessionWrapper(session), new Native.ServiceController());
        }

        internal static ActionResult Enable(ISession session, IServiceController controller)
        {
            try
            {
                // Core Agent last: an SCM start must not race enabling procmgr.
                foreach (var name in ProtectedServices.Reverse())
                {
                    if (!controller.ServiceExists(name))
                    {
                        continue;
                    }
                    var startType = name == Constants.AgentServiceName ? ServiceStartMode.Automatic : ServiceStartMode.Manual;
                    controller.SetDelayedAutoStart(name, startType == ServiceStartMode.Automatic);
                    controller.SetStartType(name, startType);
                }
                return ActionResult.Success;
            }
            catch (Exception e)
            {
                session.Log($"Failed to enable services: {e}");
                return ActionResult.Failure;
            }
        }

        public static ActionResult Cleanup(Session session)
        {
            try
            {
                File.Delete(SnapshotPath(new SessionWrapper(session)));
            }
            catch (Exception e)
            {
                session.Log($"Failed to clean up service startup snapshot: {e}");
            }
            return ActionResult.Success;
        }
    }
}
