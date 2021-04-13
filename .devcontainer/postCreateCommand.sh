#!/bin/sh

cat >> /home/vscode/.zshrc << EOF
[ -f /go/src/github.com/StackVista/stackstate-agent/.env ] && source /go/src/github.com/StackVista/stackstate-agent/.env
EOF

pip2 install -r requirements.txt
pip2 install virtualenv
