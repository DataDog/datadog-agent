# Samba server for the SMB log source integration tests
# (pkg/logs/launchers/smb/samba_integration_test.go), which build this image
# and run it on a random local port.
#
# The base is the Docker Official alpine image, pinned by the digest of its
# multi-arch index (amd64 and arm64). Samba comes from that Alpine branch,
# pinned to its 4.23 minor version: Alpine keeps only the latest revision of a
# package, so an exact version pin would break at the next security update.
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --no-cache 'samba-server~4.23' 'samba-common-tools~4.23' \
 && adduser -D -H -s /sbin/nologin smbtest \
 && mkdir -p /srv/logs \
 && chown smbtest:smbtest /srv/logs

COPY smb.conf /etc/samba/smb.conf
RUN testparm --suppress-prompt >/dev/null

# The account has no password until the test sets one (smbpasswd, through
# docker exec with the password on stdin), so no secret is baked into the
# image or passed in its environment.
EXPOSE 445
CMD ["smbd", "--foreground", "--no-process-group", "--debug-stdout"]
