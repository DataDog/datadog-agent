#!/usr/bin/env bash

# Need to symlink /dev/kmsg
ln -s /dev/console /dev/kmsg

(trap 'kill 0' SIGINT; cri-dockerd & sleep 3; kubelet "$@")
