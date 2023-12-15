using Datadog.CustomActions.Interfaces;

namespace Datadog.CustomActions.Extensions
{
    /// <summary>
    /// Taken from WixSharp since we need it in .Net 3.5
    /// </summary>
    public static class SessionExtensions
    {
        /// <summary>
        /// Determines whether the specified <see
        /// cref="T:Microsoft.Deployment.WindowsInstaller.Session"/> is active.
        /// <para>
        /// It is useful for checking if the session is terminated (e.g. in deferred custom actions).
        /// </para>
        /// </summary>
        /// <param name="session">The session.</param>
        /// <returns></returns>
        public static bool IsActive(this ISession session)
        {
            //if (!session.IsClosed) //unfortunately isClosed is always false even for the deferred actions
            try
            {
                var test = session.Components; //it will throw for the deferred action
                var text = session["INSTALLDIR"];
                return true;
            }
            catch
            {
                return false;
            }
        }

        /// <summary>
        /// Returns the value of the named property of the specified <see
        /// cref="T:Microsoft.Deployment.WindowsInstaller.Session"/> object.
        /// <para>
        /// It can be uses as a generic way of accessing the properties as it redirects
        /// (transparently) access to the <see
        /// cref="T:Microsoft.Deployment.WindowsInstaller.Session.CustomActionData"/> if the session
        /// is terminated (e.g. in deferred custom actions).
        /// </para>
        /// </summary>
        /// <param name="session">The session.</param>
        /// <param name="name">The name.</param>
        /// <returns></returns>
        public static string Property(this ISession session, string name)
        {
            if (session.IsActive())
            {
                return session[name];
            }
            else
            {
                return (session.CustomActionData.ContainsKey(name) ? session.CustomActionData[name] : "");
            }
        }
    }
}
