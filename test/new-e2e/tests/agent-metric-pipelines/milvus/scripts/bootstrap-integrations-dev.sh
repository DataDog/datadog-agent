#!/usr/bin/env bash
# One-time bootstrap for running the Milvus scenario on the
# `agent-integrations-dev` AWS account (account id 030537971304).
#
# What it does (idempotent):
#   1. Verifies the aws-vault SSO session is live.
#   2. Creates an EC2 keypair in that account if one for $USER doesn't exist,
#      and stores the private/public key pair under ~/.ssh/.
#   3. Generates a random Pulumi passphrase the first time and persists it
#      to a small per-user state file.
#
# After this runs once, use ./scripts/up.sh / ./scripts/down.sh normally —
# _lib.sh reads the artifacts produced here.

source "$(dirname -- "${BASH_SOURCE[0]}")/_lib.sh"

# We can't call ensure_aws_setup_integrations_dev directly because it
# expects the bootstrap to have happened. Reproduce just the bits we need.

AWS_PROFILE_NAME=sso-agent-integrations-dev-account-admin
KEY_NAME="e2e-agent-integrations-dev-${USER}"
PRIVATE_KEY_PATH="${HOME}/.ssh/id_rsa_e2e_agent_integrations_dev_${USER}.pem"
PUBLIC_KEY_PATH="${PRIVATE_KEY_PATH%.pem}.pub"
STATE_DIR="${HOME}/.config/dd-agent-milvus-lab"
PASSPHRASE_FILE="${STATE_DIR}/pulumi.passphrase"

vault() { aws-vault exec "${AWS_PROFILE_NAME}" -- "$@"; }

# 1. SSO session ------------------------------------------------------------

log "checking aws-vault SSO session for ${AWS_PROFILE_NAME}..."
if ! vault aws sts get-caller-identity >/dev/null 2>&1; then
    log "  no active session; running 'aws-vault login'"
    aws-vault login "${AWS_PROFILE_NAME}"
fi
IDENT=$(vault aws sts get-caller-identity --query Arn --output text)
log "  caller: ${IDENT}"

# 2. Keypair ----------------------------------------------------------------

log "ensuring EC2 keypair '${KEY_NAME}' exists..."
mkdir -p "${HOME}/.ssh"
chmod 700 "${HOME}/.ssh"

remote_exists=$(vault aws ec2 describe-key-pairs --region us-east-1 \
    --filters "Name=key-name,Values=${KEY_NAME}" \
    --query 'KeyPairs[0].KeyName' --output text 2>/dev/null || echo None)

if [[ "${remote_exists}" == "${KEY_NAME}" && -f "${PRIVATE_KEY_PATH}" && -f "${PUBLIC_KEY_PATH}" ]]; then
    log "  ✓ keypair present locally and in AWS"
elif [[ "${remote_exists}" == "${KEY_NAME}" && ! -f "${PRIVATE_KEY_PATH}" ]]; then
    fail "AWS already has keypair '${KEY_NAME}' but ${PRIVATE_KEY_PATH} is missing. "\
"Either restore from backup, or delete the remote keypair and re-run bootstrap:\n  "\
"aws-vault exec ${AWS_PROFILE_NAME} -- aws ec2 delete-key-pair --region us-east-1 --key-name ${KEY_NAME}"
else
    if [[ -f "${PRIVATE_KEY_PATH}" ]]; then
        log "  importing existing local key as AWS keypair"
        vault aws ec2 import-key-pair --region us-east-1 \
            --key-name "${KEY_NAME}" \
            --public-key-material "fileb://${PUBLIC_KEY_PATH}" >/dev/null
    else
        log "  creating new keypair in AWS, saving to ${PRIVATE_KEY_PATH}"
        vault aws ec2 create-key-pair --region us-east-1 \
            --key-name "${KEY_NAME}" --key-type rsa \
            --query KeyMaterial --output text > "${PRIVATE_KEY_PATH}"
        chmod 600 "${PRIVATE_KEY_PATH}"
        ssh-keygen -y -f "${PRIVATE_KEY_PATH}" > "${PUBLIC_KEY_PATH}"
        chmod 644 "${PUBLIC_KEY_PATH}"
    fi
    log "  ✓ keypair ready"
fi

# 3. Pulumi passphrase ------------------------------------------------------

log "ensuring local Pulumi passphrase exists..."
mkdir -p "${STATE_DIR}"
chmod 700 "${STATE_DIR}"
if [[ ! -f "${PASSPHRASE_FILE}" ]]; then
    head -c 32 /dev/urandom | base64 > "${PASSPHRASE_FILE}"
    chmod 600 "${PASSPHRASE_FILE}"
    log "  generated new passphrase → ${PASSPHRASE_FILE}"
else
    log "  ✓ passphrase already at ${PASSPHRASE_FILE}"
fi

log "bootstrap complete"
log "  KEY_NAME            = ${KEY_NAME}"
log "  PRIVATE_KEY_PATH    = ${PRIVATE_KEY_PATH}"
log "  PUBLIC_KEY_PATH     = ${PUBLIC_KEY_PATH}"
log "  PASSPHRASE_FILE     = ${PASSPHRASE_FILE}"
log ""
log "next: ./scripts/up.sh"
