#!/bin/bash -xe

time python generate_contexts.py
time cat random_contexts.csv|sort|uniq > random_sorted_uniq_contexts.csv
time go test ./... -run TestCollisions

echo "Tested against $(wc -l ./random_sorted_uniq_contexts.csv) contexts"
