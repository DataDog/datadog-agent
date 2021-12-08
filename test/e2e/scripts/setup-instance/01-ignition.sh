#!/usr/bin/env bash
set -euo pipefail

printf '=%.0s' {0..79} ; echo
set -x

cd "$(dirname "$0")"
ssh-keygen -b 4096 -t rsa -C "datadog" -N "" -f "id_rsa"
SSH_RSA=$(cat id_rsa.pub)

case "$(uname)" in
    Linux)  fcct="fcct-$(uname -m)-unknown-linux-gnu";;
    Darwin) fcct="fcct-$(uname -m)-apple-darwin";;
esac
curl -LOC - "https://github.com/coreos/butane/releases/download/v0.6.0/${fcct}"
curl -LO    "https://github.com/coreos/butane/releases/download/v0.6.0/${fcct}.asc"

gpg --import <<EOF
-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBF1RVqsBEADWMBqYv/G1r4PwyiPQCfg5fXFGXV1FCZ32qMi9gLUTv1CX7rYy
H4Inj93oic+lt1kQ0kQCkINOwQczOkm6XDkEekmMrHknJpFLwrTK4AS28bYF2RjL
M+QJ/dGXDMPYsP0tkLvoxaHr9WTRq89A+AmONcUAQIMJg3JxXAAafBi2UszUUEPI
U35MyufFt2ePd1k/6hVAO8S2VT72TxXSY7Ha4X2J0pGzbqQ6Dq3AVzogsnoIi09A
7fYutYZPVVAEGRUqavl0th8LyuZShASZ38CdAHBMvWV4bVZghd/wDV5ev3LXUE0o
itLAqNSeiDJ3grKWN6v0qdU0l3Ya60sugABd3xaE+ROe8kDCy3WmAaO51Q880ZA2
iXOTJFObqkBTP9j9+ZeQ+KNE8SBoiH1EybKtBU8HmygZvu8ZC1TKUyL5gwGUJt8v
ergy5Bw3Q7av520sNGD3cIWr4fBAVYwdBoZT8RcsnU1PP67NmOGFcwSFJ/LpiOMC
pZ1IBvjOC7KyKEZY2/63kjW73mB7OHOd18BHtGVkA3QAdVlcSule/z68VOAy6bih
E6mdxP28D4INsts8w6yr4G+3aEIN8u0qRQq66Ri5mOXTyle+ONudtfGg3U9lgicg
z6oVk17RT0jV9uL6K41sGZ1sH/6yTXQKagdAYr3w1ix2L46JgzC+/+6SSwARAQAB
tDFGZWRvcmEgKDMyKSA8ZmVkb3JhLTMyLXByaW1hcnlAZmVkb3JhcHJvamVjdC5v
cmc+iQI4BBMBAgAiBQJdUVarAhsPBgsJCAcDAgYVCAIJCgsEFgIDAQIeAQIXgAAK
CRBsEwJtEslE0LdAD/wKdAMtfzr7O2y06/sOPnrb3D39Y2DXbB8y0iEmRdBL29Bq
5btxwmAka7JZRJVFxPsOVqZ6KARjS0/oCBmJc0jCRANFCtM4UjVHTSsxrJfuPkel
vrlNE9tcR6OCRpuj/PZgUa39iifF/FTUfDgh4Q91xiQoLqfBxOJzravQHoK9VzrM
NTOu6J6l4zeGzY/ocj6DpT+5fdUO/3HgGFNiNYPC6GVzeiA3AAVR0sCyGENuqqdg
wUxV3BIht05M5Wcdvxg1U9x5I3yjkLQw+idvX4pevTiCh9/0u+4g80cT/21Cxsdx
7+DVHaewXbF87QQIcOAing0S5QE67r2uPVxmWy/56TKUqDoyP8SNsV62lT2jutsj
LevNxUky011g5w3bc61UeaeKrrurFdRs+RwBVkXmtqm/i6g0ZTWZyWGO6gJd+HWA
qY1NYiq4+cMvNLatmA2sOoCsRNmE9q6jM/ESVgaH8hSp8GcLuzt9/r4PZZGl5CvU
eldOiD221u8rzuHmLs4dsgwJJ9pgLT0cUAsOpbMPI0JpGIPQ2SG6yK7LmO6HFOxb
Akz7IGUt0gy1MzPTyBvnB+WgD1I+IQXXsJbhP5+d+d3mOnqsd6oDM/grKBzrhoUe
oNadc9uzjqKlOrmrdIR3Bz38SSiWlde5fu6xPqJdmGZRNjXtcyJlbSPVDIloxw==
=QWRO
-----END PGP PUBLIC KEY BLOCK-----
EOF

gpg --verify "${fcct}.asc" "$fcct"
chmod +x "$fcct"

"./$fcct" --pretty --strict <<EOF | tee ignition.json
variant: fcos
version: 1.1.0
passwd:
  users:
    - name: core
      ssh_authorized_keys:
        - "${SSH_RSA}"
systemd:
  units:
    - name: zincati.service
      mask: true
    - name: terminate.service
      contents: |
        [Unit]
        Description=Trigger a poweroff

        [Service]
        ExecStart=/bin/systemctl poweroff
        Restart=no
    - name: terminate.timer
      enabled: true
      contents: |
        [Timer]
        OnBootSec=7200

        [Install]
        WantedBy=multi-user.target
storage:
  links:
    - path: /etc/crypto-policies/back-ends/opensshserver.config
      target: /usr/share/crypto-policies/LEGACY/opensshserver.txt
      overwrite: true
  files:
    - path: /etc/ssh/sshd_config.d/99-datadog.conf
      mode: 0400
      contents:
        source: "data:,AcceptEnv%20DOCKER%5FREGISTRY%5F%2A%20DATADOG%5F%2AAGENT%5FIMAGE%20ARGO%5FWORKFLOW%20DATADOG%5FAGENT%5F%2A%5FKEY%20DATADOG%5FAGENT%5FSITE%0A"
EOF
