#!/bin/sh

# noop loader for heroku
# shouldn't be needed in practice, but it is used by the systemd service,
# which is used when installing the deb manually

shift # remove the path to the configuration file
exec "$@"
