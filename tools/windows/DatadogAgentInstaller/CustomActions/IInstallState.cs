namespace Datadog.CustomActions;

public interface IInstallState
{
    public bool FirstInstall
    {
        get;
    }

    public bool Upgrading
    {
        get;
    }

    public bool Uninstalling
    {
        get;
    }

    public bool Maintenance
    {
        get;
    }

    public bool RemovingForUpgrade
    {
        get;
    }
}
