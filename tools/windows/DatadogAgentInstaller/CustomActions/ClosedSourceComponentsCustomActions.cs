using System;
using Datadog.CustomActions.Extensions;
using Datadog.CustomActions.Interfaces;
using Datadog.CustomActions.Native;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions
{
    public class ClosedSourceComponentsCustomActions
    {
        private readonly ISession _session;
        private readonly IRegistryServices _registryServices;

        public ClosedSourceComponentsCustomActions(
            ISession session,
            IRegistryServices registryServices)
        {
            _session = session;
            _registryServices = registryServices;
        }

        public ClosedSourceComponentsCustomActions(ISession session)
            : this(
                session,
                new RegistryServices())
        {
        }

        /// <summary>
        /// Contains backwards compatibility logic for setting the ALLOWCLOSEDSOURCE property.
        /// </summary>
        /// <remarks>
        /// Must be called before ReadInstallationData() so that providing ALLOWCLOSEDSOURCE on the command line
        /// takes precedence over the backwards compatibility logic here, and so the backwards compatability logic
        /// can take precedence over the registry state.
        /// </remarks>
        public ActionResult ProcessAllowClosedSource()
        {
            try
            {
                bool allowClosedSource = false;

                var npm = _session.Feature("NPM");
                _session.Log($"NPM Feature: {npm.CurrentState} -> {npm.RequestState}");

                using var subkey = _registryServices.OpenRegistryKey(Registries.LocalMachine, Constants.DatadogAgentRegistryKey);
                var allowClosedSourceFromReg = subkey.GetValue(Constants.AllowClosedSourceRegistryKey)?.ToString();
                _session.Log($"ALLOWCLOSEDSOURCE registry key: {allowClosedSourceFromReg}");

                _session.Log($"ADDLOCAL: {_session.Property("ADDLOCAL")}");
                _session.Log($"NPM: {_session.Property("NPM")}");

                var allowClosedSourceFromCli = _session.Property("ALLOWCLOSEDSOURCE");
                if (string.IsNullOrEmpty(allowClosedSourceFromCli))
                {
                    // ALLOWCLOSEDSOURCE was not provided on the command line.
                    // If the customer is requesting to install NPM, or already has it installed, then set ALLOWCLOSEDSOURCE.
                    allowClosedSource =
                        allowClosedSourceFromReg == Constants.AllowClosedSource_Yes ||
                        npm.RequestState == InstallState.Local ||
                        !string.IsNullOrEmpty(_session.Property("NPM"));
                }
                else
                {
                    allowClosedSource = allowClosedSourceFromCli == Constants.AllowClosedSource_Yes;
                }

                if (allowClosedSource)
                {
                    _session["ALLOWCLOSEDSOURCE"] = Constants.AllowClosedSource_Yes;

                    // Set another property that will control the state of the Allow Closed Source checkbox in the GUI
                    // We cannot use the same property because the checkbox interprets any property values as "checked",
                    // but we need the property to be either "0" or "1" in order for the WiX RegistryValue element to
                    // write the value to the registry, it will fail if the property is not set. So we must either add
                    // another custom action or another property.
                    _session["CHECKBOX_ALLOWCLOSEDSOURCE"] = Constants.AllowClosedSource_Yes;
                }
                else
                {
                    // Ensure we always set this property, otherwise the RegistryValue action will fail
                    _session["ALLOWCLOSEDSOURCE"] = Constants.AllowClosedSource_No;
                }
            }
            catch (Exception e)
            {
                _session.Log($"Error reading install state: {e}");
                return ActionResult.Failure;
            }

            return ActionResult.Success;
        }

        [CustomAction]
        public static ActionResult ProcessAllowClosedSource(Session session)
        {
            return new ClosedSourceComponentsCustomActions(new SessionWrapper(session)).ProcessAllowClosedSource();
        }
    }
}
