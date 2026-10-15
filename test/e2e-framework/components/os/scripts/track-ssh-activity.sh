#!/bin/bash

# Keep the "last-activity" tag of this EC2 instance up to date while it is being used over SSH.
# test-infra-cleaner (aws-nuke) does not delete instances whose "last-activity" tag is recent,
# so instances that are still in use survive past their regular lifetime.
#
# The tag is refreshed when an SSH session opens (PAM hook) and periodically while at least one
# SSH session is active (systemd timer). Everything here is best effort: a failure must never
# block SSH logins nor the rest of the user data.

cat >/usr/local/bin/dd-e2e-track-ssh-activity <<'EOF'
#!/bin/bash
# Usage: dd-e2e-track-ssh-activity [--if-active]
#   --if-active: only update the tag if an SSH session is currently open

set -u

TAG_KEY="last-activity"
STAMP_FILE="/run/dd-e2e-last-activity"
IMDS="http://169.254.169.254/latest"

# Called by pam_exec: only act on session open, and never block the login
if [ -n "${PAM_TYPE:-}" ]; then
	[ "$PAM_TYPE" = "open_session" ] || exit 0
	PAM_TYPE="" setsid "$0" </dev/null >/dev/null 2>&1 &
	exit 0
fi

if [ "${1:-}" = "--if-active" ] && ! pgrep -f '^sshd(-session)?: [^ ]+@' >/dev/null 2>&1; then
	exit 0
fi

# Throttle: tests open many SSH connections in a row, one update per minute is enough
if [ -n "$(find "$STAMP_FILE" -mmin -1 2>/dev/null)" ]; then
	exit 0
fi
touch "$STAMP_FILE"

imds_token=$(curl -sf -m 2 -X PUT -H "X-aws-ec2-metadata-token-ttl-seconds: 300" "$IMDS/api/token") || exit 0
imds() { curl -sf -m 2 -H "X-aws-ec2-metadata-token: $imds_token" "$IMDS/meta-data/$1"; }

instance_id=$(imds instance-id) || exit 0
region=$(imds placement/region) || exit 0
role=$(imds iam/security-credentials/) || exit 0
creds=$(imds "iam/security-credentials/$role") || exit 0
json_field() { printf '%s\n' "$creds" | sed -n "s/.*\"$1\" *: *\"\([^\"]*\)\".*/\1/p"; }
access_key=$(json_field AccessKeyId)
secret_key=$(json_field SecretAccessKey)
session_token=$(json_field Token)

# Sign an EC2 CreateTags call with SigV4 using only curl and openssl, which are available on
# every AMI we use, unlike the AWS CLI.
sha256() { sha256sum | cut -d' ' -f1; }
hmac() { printf '%s' "$2" | openssl dgst -sha256 -mac HMAC -macopt "$1" | sed 's/^.*= //'; }

now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
amz_date=$(date -u +%Y%m%dT%H%M%SZ)
date_stamp=${amz_date%%T*}
host="ec2.$region.amazonaws.com"
content_type="application/x-www-form-urlencoded; charset=utf-8"
body="Action=CreateTags&ResourceId.1=$instance_id&Tag.1.Key=$TAG_KEY&Tag.1.Value=${now//:/%3A}&Version=2016-11-15"

signed_headers="content-type;host;x-amz-date;x-amz-security-token"
canonical_request=$(printf 'POST\n/\n\ncontent-type:%s\nhost:%s\nx-amz-date:%s\nx-amz-security-token:%s\n\n%s\n%s' \
	"$content_type" "$host" "$amz_date" "$session_token" "$signed_headers" "$(printf '%s' "$body" | sha256)")
scope="$date_stamp/$region/ec2/aws4_request"
string_to_sign=$(printf 'AWS4-HMAC-SHA256\n%s\n%s\n%s' "$amz_date" "$scope" "$(printf '%s' "$canonical_request" | sha256)")

k_date=$(hmac "key:AWS4$secret_key" "$date_stamp")
k_region=$(hmac "hexkey:$k_date" "$region")
k_service=$(hmac "hexkey:$k_region" "ec2")
k_signing=$(hmac "hexkey:$k_service" "aws4_request")
signature=$(hmac "hexkey:$k_signing" "$string_to_sign")

# Pass the credentials through a config on stdin rather than the command line to keep them out of ps
printf 'header = "Authorization: AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s"\nheader = "X-Amz-Security-Token: %s"\n' \
	"$access_key" "$scope" "$signed_headers" "$signature" "$session_token" |
	curl -sf -m 10 -K - -o /dev/null \
		-H "Content-Type: $content_type" \
		-H "X-Amz-Date: $amz_date" \
		--data "$body" \
		"https://$host/"
EOF
chmod 0755 /usr/local/bin/dd-e2e-track-ssh-activity || true

if [ -f /etc/pam.d/sshd ] && ! grep -q dd-e2e-track-ssh-activity /etc/pam.d/sshd; then
	echo "session optional pam_exec.so quiet /usr/local/bin/dd-e2e-track-ssh-activity" >>/etc/pam.d/sshd || true
fi

if command -v systemctl >/dev/null 2>&1; then
	cat >/etc/systemd/system/dd-e2e-track-ssh-activity.service <<'EOF'
[Unit]
Description=Update the last-activity EC2 tag while SSH sessions are open

[Service]
Type=oneshot
ExecStart=/usr/local/bin/dd-e2e-track-ssh-activity --if-active
EOF
	cat >/etc/systemd/system/dd-e2e-track-ssh-activity.timer <<'EOF'
[Unit]
Description=Periodically update the last-activity EC2 tag while SSH sessions are open

[Timer]
OnBootSec=1min
OnUnitActiveSec=5min

[Install]
WantedBy=timers.target
EOF
	systemctl daemon-reload || true
	systemctl enable --now dd-e2e-track-ssh-activity.timer || true
fi
