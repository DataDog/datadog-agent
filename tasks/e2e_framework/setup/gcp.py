import getpass
import json
from pathlib import Path

from invoke.context import Context
from invoke.exceptions import Exit

from tasks.e2e_framework.config import Config
from tasks.e2e_framework.setup.ssh_keys import add_key_to_ssh_agent, default_key_paths, generate_keypair_with_passphrase
from tasks.e2e_framework.tool import info


def setup_gcp_config(ctx: Context, config: Config):
    if config.configParams.gcp is None:
        config.configParams.gcp = Config.Params.GCP(publicKeyPath=None)

    gcp = config.configParams.gcp
    user = getpass.getuser()

    if not gcp.account:
        gcp.account = "agent-sandbox"
    info(f"✓ GCP account: {gcp.account}")

    default_priv, default_pub = default_key_paths(gcp.account, user, provider="gcp", key_type="ed25519")

    if not gcp.privateKeyPath:
        gcp.privateKeyPath = str(default_priv)
    if not gcp.publicKeyPath:
        gcp.publicKeyPath = str(default_pub)

    private_path = Path(gcp.privateKeyPath).expanduser()
    public_path = Path(gcp.publicKeyPath).expanduser()

    if not private_path.is_file():
        info(f"🔑 Generating GCP SSH keypair → {private_path}")
        passphrase = generate_keypair_with_passphrase(ctx, str(private_path), str(public_path), key_type="ed25519")
        gcp.privateKeyPassword = passphrase
        info("✓ GCP SSH key encrypted with passphrase (stored in ~/.test_infra_config.yaml, chmod 0600)")
        add_key_to_ssh_agent(ctx, str(private_path), passphrase)
    else:
        info(f"✓ GCP SSH keypair present: {private_path}")

    if gcp.pullSecretPath is None:
        gcp.pullSecretPath = ""


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
