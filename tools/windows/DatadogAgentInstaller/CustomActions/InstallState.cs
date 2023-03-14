using System;
using Datadog.CustomActions.Extensions;

namespace Datadog.CustomActions;

/// <summary>
/// Mirror of <see cref="WixSetup.Conditions" /> but for runtime.
/// </summary>
public class InstallState : IInstallState
{
    private readonly ISession _session;

    public InstallState(ISession session)
    {
        _session = session;
    }

    public bool Installed => !string.IsNullOrEmpty(_session.Property("Installed"));
    public bool WixUpgradeDetected => !string.IsNullOrEmpty(_session.Property("WIX_UPGRADE_DETECTED"));
    public bool HasUpgradingProductCode => !string.IsNullOrEmpty(_session.Property("UPGRADINGPRODUCTCODE"));
    public bool BeingUninstalled => _session.Property("REMOVE").Equals("All", StringComparison.InvariantCultureIgnoreCase);
    public bool FirstInstall => !Installed && !WixUpgradeDetected;
    public bool Upgrading => WixUpgradeDetected && !BeingUninstalled;
    public bool Uninstalling => Installed && !BeingUninstalled && !(WixUpgradeDetected || HasUpgradingProductCode);
    public bool Maintenance => Installed && !Upgrading && !Uninstalling && !HasUpgradingProductCode;
    public bool RemovingForUpgrade => BeingUninstalled && HasUpgradingProductCode;
}
