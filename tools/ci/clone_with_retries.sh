#!/bin/bash

if [ "$#" -lt 1 ]; then
    print "usage: $0 [options] <repository to clone>"
    print "The script will forward all options to git clone"
    exit 1
fi

for i in $(seq 1 5); do
    git clone "$@" && exit 0;
    sleep $((i ** 2))
done

exit 1
