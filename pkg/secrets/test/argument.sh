#!/bin/bash


if [ "$1" != "arg1" ]
then
	exit 1
fi

if [ "$2" != "arg2" ]
then
	exit 1
fi

echo -n '{"handle1":{"value":"arg_password"}}'
