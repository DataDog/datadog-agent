#!/bin/bash

input=`cat`

if [[ "$input" == "{\"version\": \"1.0\" , \"secrets\": [\"sec1\", \"sec2\"]}" ]]
then
	echo -n '{"handle1":{"value":"input_password"}}'
else
	exit 1
fi
