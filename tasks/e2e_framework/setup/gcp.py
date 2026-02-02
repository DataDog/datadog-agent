import json
import os
from pathlib import Path

from invoke.exceptions import Exit

from tasks.e2e_framework.config import Config
from tasks.e2e_framework.tool import ask, warn


def setup_gcp_config(config: Config):
    if config.configParams is None:
        config.configParams = Config.Params(aws=None, agent=None, pulumi=None, azure=None, gcp=None)
    if config.configParams.gcp is None:
        config.configParams.gcp = Config.Params.GCP(publicKeyPath=None)

    # gcp public key path
    if config.configParams.gcp.publicKeyPath is None:
        config.configParams.gcp.publicKeyPath = str(Path.home().joinpath(".ssh", "id_ed25519.pub").absolute())
    default_public_key_path = config.configParams.gcp.publicKeyPath
    while True:
        config.configParams.gcp.publicKeyPath = default_public_key_path
        public_key_path = ask(
            f"🔑 Path to your GCP public ssh key: (default: [{config.configParams.gcp.publicKeyPath}])"
        )
        if public_key_path:
            config.configParams.gcp.publicKeyPath = public_key_path

        if os.path.isfile(config.configParams.gcp.publicKeyPath):
            break
        warn(f"{config.configParams.gcp.publicKeyPath} is not a valid ssh key")

    default_account = ask(f"🔑 Default account to use, default [{config.configParams.gcp.account}]: ")
    if default_account:
        config.configParams.gcp.account = default_account

    # openShift pull secret path
    if config.configParams.gcp.pullSecretPath is None:
        config.configParams.gcp.pullSecretPath = ""
    default_pull_secret_path = config.configParams.gcp.pullSecretPath
    while True:
        config.configParams.gcp.pullSecretPath = default_pull_secret_path
        pull_secret_path = ask("🔑 Path to your OpenShift pull secret file (optional, can be set later): ")
        if not pull_secret_path:
            # empty to skip
            config.configParams.gcp.pullSecretPath = ""
            break

        config.configParams.gcp.pullSecretPath = pull_secret_path
        if os.path.isfile(config.configParams.gcp.pullSecretPath):
            break
        warn(f"{config.configParams.gcp.pullSecretPath} is not a valid file")


# Check if gke-gcloud-auth-plugin is installed and install it if not
def install_gcloud_auth_plugin(ctx):
    res = ctx.run('gcloud components list --format=json --filter "name: gke-gcloud-auth-plugin"', hide=True)
    installed_component = json.loads(res.stdout)
    if installed_component[0]["state"]["name"] == "Installed":
        print("✅ gke-gcloud-auth-plugin is already installed")
        return
    print("🤖 Installing gke-gcloud-auth-plugin")
    install = ctx.run("gcloud components install -q gke-gcloud-auth-plugin", hide=True)
    if install is None:
        raise Exit("Failed to install gke-gcloud-auth-plugin")
    if install.exited != 0:
        raise Exit(f"Failed to install gke-gcloud-auth-plugin: {install.stderr}")
    print("✅ gke-gcloud-auth-plugin installed")
