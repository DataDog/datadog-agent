#!/bin/sh

echo "$1" | s6-tai64n | s6-tai64nlocal >> /var/log/datadog/init.log
echo "$1"
