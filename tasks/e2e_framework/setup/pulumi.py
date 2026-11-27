import secrets

from tasks.e2e_framework.config import Config
from tasks.e2e_framework.tool import info

# Length matches Pulumi cloud-generated passphrases. token_urlsafe yields ~43 chars from 32 bytes.
_PASSPHRASE_BYTES = 32


def setup_pulumi_config(config: Config):
    """
    Apply silent defaults for the Pulumi config block: pick a sensible log level,
    set logToStdErr on, and generate a random passphrase if none exists.

    Re-running is safe: existing values are preserved.
    """
    if config.configParams.pulumi is None:
        config.configParams.pulumi = Config.Params.Pulumi()

    pulumi = config.configParams.pulumi

    if pulumi.logLevel is None:
        pulumi.logLevel = 1
    if pulumi.logToStdErr is None:
        pulumi.logToStdErr = True

    if pulumi.verboseProgressStreams is None:
        pulumi.verboseProgressStreams = True

    if not pulumi.passphrase:
        pulumi.passphrase = secrets.token_urlsafe(_PASSPHRASE_BYTES)
        info("✓ Generated Pulumi passphrase (stored in ~/.test_infra_config.yaml, chmod 0600)")
    else:
        info("✓ Pulumi passphrase already configured")
