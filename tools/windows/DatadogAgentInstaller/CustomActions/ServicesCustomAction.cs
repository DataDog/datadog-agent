using System;
using System.ServiceProcess;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions
{
    public class ServicesCustomAction
    {
        private static ActionResult StartServices(ISession session)
        {
            try
            {
                var service = new ServiceController("datadogagent");
                service.Start();
                service.WaitForStatus(ServiceControllerStatus.Running, TimeSpan.FromSeconds(30));
            }
            catch (Exception e)
            {
                session.Log($"Failed to start service: {e}");
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult StartServices(Session session)
        {
            return StartServices(new SessionWrapper(session));
        }
    }
}
