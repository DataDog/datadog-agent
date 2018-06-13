#!/bin/bash

printf '=%.0s' {0..79} ; echo

for f in /run/systemd/resolve/resolv.conf /etc/resolv.conf /etc/hosts /etc/os-release
do
    echo ${f}
    echo "---"
    cat ${f}
    printf '=%.0s' {0..79} ; echo
done

_wait_binary() {
    echo "waiting for $1 binary to be in PATH=${PATH} ..."
    for i in {0..120}
    do
        which $1 2> /dev/null && break
        sleep 1
    done
}

_wait_binary pupernetes

# Binary is here, so setup-pupernetes has completed
# pupernetes.service should start soon because it contains the constraint After=setup-pupernetes.service

set -x
sudo -kE pupernetes wait --unit-to-watch pupernetes.service --logging-since 2h --timeout 20m || {
    # Here pupernetes.service may not be started yet and be considered as dead by go-systemd
    # A single retry is enough
    # https://github.com/DataDog/pupernetes/issues/46
    sleep 10
    sudo -kE pupernetes wait --unit-to-watch pupernetes.service --logging-since 2h --timeout 20m
}

exit $?
