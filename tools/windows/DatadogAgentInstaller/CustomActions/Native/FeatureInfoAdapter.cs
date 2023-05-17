using System;
using System.Collections.Generic;
using System.Linq;
using System.Text;
using System.Threading.Tasks;
using Datadog.CustomActions.Interfaces;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions.Native
{
    /// <summary>
    /// Wraps the <see cref="FeatureInfo"/> from the Windows installer.
    /// </summary>
    public class FeatureInfoAdapter : IFeatureInfo
    {
        private FeatureInfo _featureInfo;

        public FeatureInfoAdapter(FeatureInfo featureInfo)
        {
            _featureInfo = featureInfo;
        }

        public InstallState RequestState => _featureInfo.RequestState;
        public InstallState CurrentState => _featureInfo.CurrentState;
    }
}
