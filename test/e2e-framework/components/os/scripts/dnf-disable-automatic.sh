#!/bin/bash
systemctl disable dnf-automatic.timer || true
systemctl stop dnf-automatic.timer || true
