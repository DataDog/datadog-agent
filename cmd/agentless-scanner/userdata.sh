#!/bin/bash

set -e

SCANNER_NAME="$1"
if [ -z "$SCANNER_NAME" ]; then
  >&2 printf "Please provide a name for the scanner\n"
  exit 1
fi

apt update
apt install -y nbd-client

modprobe nbd nbds_max=128
echo "nbd" > /etc/modules-load.d//nbd.conf
echo "options nbd nbds_max=128" > /etc/modprobe.d/nbd.conf

echo "agentless-scanning-${SCANNER_NAME}" > /etc/hostname

# Install the agent
DD_API_KEY="${DD_API_KEY}" DD_SITE="datad0g.com" DD_HOSTNAME="agentless-scanning-${SCANNER_NAME}" DD_REPO_URL="datad0g.com" DD_AGENT_DIST_CHANNEL="beta" DD_AGENT_MINOR_VERSION="50.0~agentless~scanner~2024010901" bash -c "$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh)"

cat << EOF > /etc/datadog-agent/datadog.yaml
hostname: agentless-scanning-${SCANNER_NAME}
api_key: ${DD_API_KEY}
site: datad0g.com

logs_enabled: true
log_level: info

ec2_prefer_imdsv2: true

# Remote configuration keys for staging
remote_configuration:
  enabled: true
  config_root: '{"signatures":[{"keyid":"6aac6a51efedb4e54915bf9fbd2cfb49fbf428d46052bcaf3c72409c33ecdf5e","sig":"4af18f0919fb9b8ba7ffc9f6fb325c887083c28a474981e29ccc5bdeea7a2bf2f8568be8f8bd3c6c498dd118e2c8f713d22032196cf400465f8fb700ba800f0d"},{"keyid":"bd3ea764afdf757f07bab1e9e501a5fda1d49a8da3eaddc53a50dbe2aff92545","sig":"2e6bb516308fd8c79faff015a443b65dea0af780842aacc5c05f49ae8fd709bfdd70e191a38d0b64aad03bb4398052b82bd224d6e55c90d4c38220aa9db62705"}],"signed":{"_type":"root","consistent_snapshot":true,"expires":"1970-01-01T00:00:00Z","keys":{"6aac6a51efedb4e54915bf9fbd2cfb49fbf428d46052bcaf3c72409c33ecdf5e":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"09402247ef6252018e52c7ba6a3a484936f14dad6ae921c556a1d092f4a68f0f"},"scheme":"ed25519"},"bd3ea764afdf757f07bab1e9e501a5fda1d49a8da3eaddc53a50dbe2aff92545":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"cf248bc222a5dfc9676a2a3ef90526c84adb09649db56686705f69f42908d7d8"},"scheme":"ed25519"}},"roles":{"root":{"keyids":["bd3ea764afdf757f07bab1e9e501a5fda1d49a8da3eaddc53a50dbe2aff92545","6aac6a51efedb4e54915bf9fbd2cfb49fbf428d46052bcaf3c72409c33ecdf5e"],"threshold":2},"snapshot":{"keyids":["bd3ea764afdf757f07bab1e9e501a5fda1d49a8da3eaddc53a50dbe2aff92545","6aac6a51efedb4e54915bf9fbd2cfb49fbf428d46052bcaf3c72409c33ecdf5e"],"threshold":2},"targets":{"keyids":["bd3ea764afdf757f07bab1e9e501a5fda1d49a8da3eaddc53a50dbe2aff92545","6aac6a51efedb4e54915bf9fbd2cfb49fbf428d46052bcaf3c72409c33ecdf5e"],"threshold":2},"timestamp":{"keyids":["bd3ea764afdf757f07bab1e9e501a5fda1d49a8da3eaddc53a50dbe2aff92545","6aac6a51efedb4e54915bf9fbd2cfb49fbf428d46052bcaf3c72409c33ecdf5e"],"threshold":2}},"spec_version":"1.0","version":1}}'
  director_root: '{"signatures":[{"keyid":"233a529fe7c63b5b9081f6e0e2681cc227f85e04ad434d0a165a2f69b87255a6","sig":"6d7ddf4bcbd1ce223b5352cae4671ef42800d79f0c94dda905cf0dd8a6198ba69795a19201dc7230e4bd872cf109e827233678bf76389910933472417488320e"},{"keyid":"6ca796e7b4883af3bb3d522dc0009984dcbf5ad2a6c9ea354d30acc32d8b75d1","sig":"a1236d12903e1c4024fc6340c50a0f2fe9972e967eb2bace8d6594e156f0466f772bfc0c9f30e07067904073c0d7ba7d48ad00341405312daf0d7bc502ccc50f"}],"signed":{"_type":"root","consistent_snapshot":true,"expires":"1970-01-01T00:00:00Z","keys":{"233a529fe7c63b5b9081f6e0e2681cc227f85e04ad434d0a165a2f69b87255a6":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"f7c278f32e69ce7d5ca5b81bd2cbe2b4b44177eee36ed025ec06bd19e47eaefe"},"scheme":"ed25519"},"6ca796e7b4883af3bb3d522dc0009984dcbf5ad2a6c9ea354d30acc32d8b75d1":{"keyid_hash_algorithms":["sha256","sha512"],"keytype":"ed25519","keyval":{"public":"47be15ec10499208aa5ef9a1e32010cc05c047a98d18ad084d6e4e51baa1b93c"},"scheme":"ed25519"}},"roles":{"root":{"keyids":["6ca796e7b4883af3bb3d522dc0009984dcbf5ad2a6c9ea354d30acc32d8b75d1","233a529fe7c63b5b9081f6e0e2681cc227f85e04ad434d0a165a2f69b87255a6"],"threshold":2},"snapshot":{"keyids":["6ca796e7b4883af3bb3d522dc0009984dcbf5ad2a6c9ea354d30acc32d8b75d1","233a529fe7c63b5b9081f6e0e2681cc227f85e04ad434d0a165a2f69b87255a6"],"threshold":2},"targets":{"keyids":["6ca796e7b4883af3bb3d522dc0009984dcbf5ad2a6c9ea354d30acc32d8b75d1","233a529fe7c63b5b9081f6e0e2681cc227f85e04ad434d0a165a2f69b87255a6"],"threshold":2},"timestamp":{"keyids":["6ca796e7b4883af3bb3d522dc0009984dcbf5ad2a6c9ea354d30acc32d8b75d1","233a529fe7c63b5b9081f6e0e2681cc227f85e04ad434d0a165a2f69b87255a6"],"threshold":2}},"spec_version":"1.0","version":1}}'
EOF

if [ -n "${DD_API_KEY_DUAL}" ]; then
  cat << EOF >> /etc/datadog-agent/datadog.yaml
# Dual shipping metrics and logs
additional_endpoints:
  "https://app.datadoghq.com":
  - "${DD_API_KEY_DUAL}"

logs_config:
  use_http: true
  additional_endpoints:
  - api_key: "${DD_API_KEY_DUAL}"
    Host: "agent-http-intake.logs.datadoghq.com"
    Port: 443
    is_reliable: true
EOF
fi

# Adding automatic reboot on kernel updates
cat << EOF >> /etc/apt/apt.conf.d/50unattended-upgrades
Unattended-Upgrade::Automatic-Reboot "true";
Unattended-Upgrade::Automatic-Reboot-WithUsers "true";
Unattended-Upgrade::Automatic-Reboot-Time "now";
EOF

# Activate agentless-scanner logging
mkdir -p /etc/datadog-agent/conf.d/agentless-scanner.d
cat <<EOF > /etc/datadog-agent/conf.d/agentless-scanner.d/conf.yaml
logs:
  - type: file
    path: "/var/log/datadog/agentless-scanner.log"
    service: "agentless-scanner"
    source: "datadog-agent"
EOF

chown -R dd-agent: /etc/datadog-agent/conf.d/agentless-scanner.d

# Restart the agent
service datadog-agent restart
