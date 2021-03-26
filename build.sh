#!/bin/bash
python3 -m venv venv3
inv deps
source venv3/bin/activate
pip3 install -r requirements.txt
inv rtloader.make --install-prefix=$PWD/dev
inv rtloader.install
inv -e agent.build --exclude-rtloader
