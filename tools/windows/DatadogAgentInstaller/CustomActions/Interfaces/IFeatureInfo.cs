using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading.Tasks;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions.Interfaces
{
    public interface IFeatureInfo
    {
        InstallState CurrentState { get; }
        InstallState RequestState { get; }
    }
}
