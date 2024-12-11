namespace Datadog.CustomActions.Interfaces
{
    interface IRollbackAction
    {
        public void Restore(ISession session, IFileSystemServices fileSystemServices, IServiceController serviceController);
    }
}
