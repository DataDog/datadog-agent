import getpass
from pathlib import Path

from invoke.context import Context

from tasks.e2e_framework.config import Config
from tasks.e2e_framework.setup.ssh_keys import add_key_to_ssh_agent, default_key_paths, generate_keypair_with_passphrase
from tasks.e2e_framework.tool import info


def setup_azure_config(ctx: Context, config: Config):
    if config.configParams.azure is None:
        config.configParams.azure = Config.Params.Azure(publicKeyPath=None)

    azure = config.configParams.azure
    user = getpass.getuser()

    if not azure.account:
        azure.account = "agent-sandbox"
    info(f"✓ Azure account: {azure.account}")

    default_priv, default_pub = default_key_paths(azure.account, user, provider="azure", key_type="ed25519")

    if not azure.privateKeyPath:
        azure.privateKeyPath = str(default_priv)
    if not azure.publicKeyPath:
        azure.publicKeyPath = str(default_pub)

    private_path = Path(azure.privateKeyPath).expanduser()
    public_path = Path(azure.publicKeyPath).expanduser()

    if not private_path.is_file():
        info(f"🔑 Generating Azure SSH keypair → {private_path}")
        passphrase = generate_keypair_with_passphrase(ctx, str(private_path), str(public_path), key_type="ed25519")
        azure.privateKeyPassword = passphrase
        info("✓ Azure SSH key encrypted with passphrase (stored in ~/.test_infra_config.yaml, chmod 0600)")
        add_key_to_ssh_agent(ctx, str(private_path), passphrase)
    else:
        info(f"✓ Azure SSH keypair present: {private_path}")
