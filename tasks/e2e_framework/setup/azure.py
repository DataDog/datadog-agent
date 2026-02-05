import os
from pathlib import Path

from tasks.e2e_framework.config import Config
from tasks.e2e_framework.tool import ask, warn


def setup_azure_config(config: Config):
    if config.configParams is None:
        config.configParams = Config.Params(aws=None, agent=None, pulumi=None, azure=None, gcp=None)
    if config.configParams.azure is None:
        config.configParams.azure = Config.Params.Azure(publicKeyPath=None)

    # azure public key path
    if config.configParams.azure.publicKeyPath is None:
        config.configParams.azure.publicKeyPath = str(Path.home().joinpath(".ssh", "id_ed25519.pub").absolute())
    default_public_key_path = config.configParams.azure.publicKeyPath
    while True:
        config.configParams.azure.publicKeyPath = default_public_key_path
        public_key_path = ask(
            f"🔑 Path to your Azure public ssh key: (default: [{config.configParams.azure.publicKeyPath}])"
        )
        if public_key_path:
            config.configParams.azure.publicKeyPath = public_key_path

        if os.path.isfile(config.configParams.azure.publicKeyPath):
            break
        warn(f"{config.configParams.azure.publicKeyPath} is not a valid ssh key")

    default_account = ask(f"🔑 Default account to use, default [{config.configParams.azure.account}]: ")
    if default_account:
        config.configParams.azure.account = default_account
