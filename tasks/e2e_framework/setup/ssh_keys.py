import base64
import os
import secrets
from pathlib import Path
from typing import NamedTuple

from invoke.context import Context
from invoke.exceptions import UnexpectedExit

from tasks.e2e_framework.tool import info, is_windows, warn


def ssh_fingerprint_to_bytes(fingerprint: str) -> bytes:
    out = fingerprint.strip().split(' ')[1]
    if out.count(':') > 1:
        # EXAMPLE: MD5(stdin)= 81:e4:46:e9:dd:a6:3d:41:6d:ca:94:21:5c:e5:1d:24
        # EXAMPLE: 2048 MD5:19:b3:a8:5f:13:7e:b9:d3:6c:75:20:d6:18:7f:e2:1d no comment (RSA)
        if out.startswith('MD5') or out.startswith('SHA'):
            out = out.split(':', 1)[1]
        return bytes.fromhex(out.replace(':', ''))
    # EXAMPLE: 256 SHA1:41jsg4Z9lgylj6/zmhGxtZ6/qZs testname (ED25519)
    # ssh leaves out padding but python will ignore extra padding so add the missing padding
    out = out.split(':', 1)
    return base64.b64decode(out[1] + '==')


# noqa: because vulture thinks this is unused
class KeyFingerprint(NamedTuple):
    md5: bytes  # noqa
    sha1: bytes  # noqa
    sha256: bytes  # noqa
    ssh_keygen: bytes  # noqa
    md5_import: bytes  # noqa


class KeyInfo(NamedTuple('KeyFingerprint', [('path', str), ('fingerprint', KeyFingerprint), ('is_rsa_pubkey', bool)])):
    def in_ssh_agent(self, ctx):
        out = ctx.run("ssh-add -l", hide=True)
        inAgent = out.stdout.strip().split('\n')
        for line in inAgent:
            line = line.strip()
            if not line:
                continue
            out = ssh_fingerprint_to_bytes(line)
            if self.match(out):
                return True
        return False

    def match(self, fingerprint: bytes):
        for f in self.fingerprint:
            if f == fingerprint:
                return True
        return False

    def match_ec2_keypair(self, keypair):
        # EC2 uses a different fingerprint hash/format depending on the key type and the key's origin
        # https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/verify-keys.html
        ec2_fingerprint = keypair["KeyFingerprint"]
        if ':' in ec2_fingerprint:
            ec2_fingerprint = bytes.fromhex(ec2_fingerprint.replace(':', ''))
        else:
            ec2_fingerprint = base64.b64decode(ec2_fingerprint + '==')
        return self.match(ec2_fingerprint)

    @classmethod
    def from_path(cls, ctx, path):
        fingerprints = {'ssh_keygen': b'', 'md5_import': b''}
        is_rsa_pubkey = False
        with open(path, 'rb') as f:
            firstline = f.readline()
            # Make sure the key is ascii
            if b'\0' in firstline:
                raise ValueError(f"Key file {path} is not ascii, it may be in utf-16, please convert it to ascii")
            if firstline.startswith(b'ssh-rsa'):
                is_rsa_pubkey = True
            # EC2 uses a different fingerprint hash/format depending on the key type and the key's origin
            # https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/verify-keys.html
            if b'SSH' in firstline or firstline.startswith(b'ssh-'):

                def getfingerprint(fmt, path):
                    out = ctx.run(f"ssh-keygen -l -E {fmt} -f \"{path}\"", hide=True)
                    return ssh_fingerprint_to_bytes(out.stdout.strip())

            elif b'BEGIN' in firstline:

                def getfingerprint(fmt, path):
                    out = ctx.run(
                        f'openssl pkcs8 -in "{path}" -inform PEM -outform DER -topk8 -nocrypt | openssl {fmt} -c',
                        hide=True,
                    )
                    # EXAMPLE: (stdin)= e3:a8:bc:0a:3a:54:9f:b8:be:6e:75:8c:98:26:8e:3d:8e:e9:d0:69
                    out = out.stdout.strip().split(' ')[1]
                    return bytes.fromhex(out.replace(':', ''))

                # AWS calculatees its fingerprints differents for RSA keys,
                # such that the sha256 fingerprint doesn't match ssh-agent/ssh-keygen.
                # It seems like they're hashing the private key instead of the public key.
                # This also means it's not possible to match a public key to an EC2 RSA fingerprint
                # if AWS generated the private key.
                out = ctx.run(f"ssh-keygen -l -f {path}", hide=True)
                fingerprints['ssh_keygen'] = ssh_fingerprint_to_bytes(out.stdout.strip())
                # If the key was imported to AWS, the fingerprint is calculated off the public key data
                out = ctx.run(
                    f"ssh-keygen -ef {path} -m PEM | openssl rsa -RSAPublicKey_in -outform DER | openssl md5 -c",
                    hide=True,
                )
                fingerprints['md5_import'] = ssh_fingerprint_to_bytes(out.stdout.strip())
            else:
                raise ValueError(f"Key file {path} is not a valid ssh key")
        # aws returns fingerprints in different formats so get a couple
        for fmt in ['md5', 'sha1', 'sha256']:
            fingerprints[fmt] = getfingerprint(fmt, path)
        return cls(path=path, fingerprint=KeyFingerprint(**fingerprints), is_rsa_pubkey=is_rsa_pubkey)


def check_key(ctx: Context, keyinfo: KeyInfo, keypair: dict, configured_keypair_name: str):
    if keypair["KeyName"] != configured_keypair_name:
        warn("WARNING: Key name does not match configured keypair name. This key will not be used for provisioning.")
    if ssh_agent_supported():
        if not keyinfo.in_ssh_agent(ctx):
            warn("WARNING: Key missing from ssh-agent. This key will not be used for connections.")
    if "rsa" not in keypair["KeyType"].lower():
        warn("WARNING: Key type is not RSA. This key cannot be used to decrypt Windows RDP credentials.")


def get_ssh_keys():
    ignore = ["known_hosts", "authorized_keys", "config"]
    root = Path.home().joinpath(".ssh")
    filenames = filter(lambda x: x.is_file() and x not in ignore, root.iterdir())
    return list(map(root.joinpath, filenames))


def passphrase_decrypts_privatekey(ctx: Context, path: str, passphrase: str):
    try:
        ctx.run(f"ssh-keygen -y -P '{passphrase}' -f {path}", hide=True)
    except UnexpectedExit as e:
        # incorrect passphrase supplied to decrypt private key
        if 'incorrect passphrase' in str(e):
            return False
    return True


def is_key_encrypted(ctx: Context, path: str):
    return not passphrase_decrypts_privatekey(ctx, path, "")


def ssh_agent_supported():
    return not is_windows()


def add_key_to_ssh_agent(ctx: Context, private_key_path: str, passphrase: str) -> None:
    """
    Add a passphrase-protected private key to the running ssh-agent non-interactively.
    On macOS, also stores the passphrase in the Keychain so the key survives reboots.
    No-op on Windows where Pulumi does not use ssh-agent.
    """
    import platform
    import stat
    import tempfile

    if not ssh_agent_supported():
        return

    # Temporary askpass script so ssh-add gets the passphrase without an interactive prompt
    with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as f:
        f.write(f'#!/bin/sh\nprintf "%s" "{passphrase}"\n')
        askpass_path = f.name
    os.chmod(askpass_path, stat.S_IRWXU)

    try:
        # On macOS, --apple-use-keychain persists the passphrase across reboots
        extra = "--apple-use-keychain " if platform.system() == "Darwin" else ""
        ctx.run(
            f'SSH_ASKPASS_REQUIRE=force SSH_ASKPASS="{askpass_path}" ssh-add {extra}"{private_key_path}"',
            hide=True,
        )
        info(f"✓ SSH key added to ssh-agent: {private_key_path}")
    except UnexpectedExit as e:
        warn(f"Could not add key to ssh-agent (is it running?): {e}")
    finally:
        os.unlink(askpass_path)


def default_key_paths(
    account: str,
    user: str,
    provider: str = "aws",
    key_type: str = "rsa",
) -> tuple[Path, Path]:
    """
    Return (private_key_path, public_key_path) for the auto-generated e2e SSH keypair.

    provider="aws"   → ~/.ssh/id_{key_type}_e2e_{account}_{user}.pem  (no prefix, backward-compat)
    provider="azure" → ~/.ssh/id_{key_type}_e2e_azure_{account}_{user}.pem
    provider="gcp"   → ~/.ssh/id_{key_type}_e2e_gcp_{account}_{user}.pem
    """
    account_part = account.replace("-", "_")
    provider_part = "" if provider == "aws" else f"{provider}_"
    private_path = Path.home() / ".ssh" / f"id_{key_type}_e2e_{provider_part}{account_part}_{user}.pem"
    public_path = private_path.with_name(f"{private_path.stem}.pub")
    return private_path, public_path


def generate_keypair_with_passphrase(
    ctx: Context,
    private_key_path: str,
    public_key_path: str,
    key_type: str = "ed25519",
) -> str:
    """
    Generate a new SSH keypair encrypted with a random passphrase.
    Writes the private key to private_key_path and the public key to public_key_path.
    Returns the passphrase.
    """
    passphrase = secrets.token_urlsafe(32)
    os.makedirs(Path(private_key_path).parent, exist_ok=True)
    ctx.run(f'ssh-keygen -t {key_type} -f "{private_key_path}" -N "{passphrase}" -C ""', hide=True)
    if not is_windows():
        os.chmod(private_key_path, 0o600)
    # ssh-keygen appends .pub to the private key path; rename to the desired public key path
    generated_pub = f"{private_key_path}.pub"
    if generated_pub != public_key_path:
        os.rename(generated_pub, public_key_path)
    return passphrase
