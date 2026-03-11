#!/bin/bash
set -e

# Only allow running this in the root directory
contents=`head -n 1 go.mod`
if [ "$contents" != "module github.com/DataDog/datadog-agent" ]; then
    # TODO: Make it work anywhere in the repo, by cd'ing to the root
    echo "Error: must be run in root directory of datadog-agent repo"
    exit 1
fi

if [ ! -f ./bin/agent/agent ]; then
    echo "Agent binary should exist already. Not found. Run 'dda inv agent.build'"
    exit 1
fi

# Make a temporary directory
cwd=`pwd`
tmpdir=`mktemp -d -t`
workdir=$tmpdir/schema-generation

# Create directories to work in
mkdir -p $workdir/phase1
mkdir -p $workdir/phase2
mkdir -p $workdir/phase3
mkdir -p $workdir/phase4

#######
# Phase 1: Run the agent command "createschema". It executes the code in
# InitConfig, which calls BindEnvAndSetDefault (and others) to generate
# the initial schema.
echo "Phase 1..."

./bin/agent/agent createschema
cp core_schema.yaml         $workdir/phase1
cp system-probe_schema.yaml $workdir/phase1

#######
# Phase 2: Get docs, env var, visibility, etc from config_template.yaml
# and use it to enrich the schema.
echo "Phase 2..."

cd pkg/config/schema

python parse_template_comment.py \
  ../config_template.yaml \
  $workdir/phase1/core_schema.yaml \
  $workdir/phase2/core_schema_enriched.yaml
python parse_template_comment.py \
  ../config_template.yaml \
  $workdir/phase1/system-probe_schema.yaml \
  $workdir/phase2/system-probe_schema_enriched.yaml

#######
# Phase 3: Additional fixes based upon a set of exceptions and one-offs
echo "Phase 3..."

cp $workdir/phase2/core_schema_enriched.yaml \
   $workdir/phase3/core_schema_enriched.yaml

cp $workdir/phase2/system-probe_schema_enriched.yaml \
   $workdir/phase3/system-probe_schema_enriched.yaml

python fix_schema.py $workdir/phase3/core_schema_enriched.yaml \
                     $workdir/phase3/system-probe_schema_enriched.yaml

#######
# Phase 4: Generate the template
echo "Phase 4..."

python ./generate_template.py \
  $workdir/phase3/core_schema_enriched.yaml \
  $workdir/phase3/system-probe_schema_enriched.yaml \
  $workdir/phase4/

cd $cwd
echo "Results in $workdir"
echo "Success!"
exit 0
