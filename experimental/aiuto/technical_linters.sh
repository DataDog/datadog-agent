#!/usr/bin/env bash
# Runs the one-line jobs from datadog-agent's .gitlab/build/lint/technical_linters.yml
# as actions of a single run_actions invocation, as a trial of consolidating
# them into one GitLab job.
#
# Usage:
#   technical_linters.sh [--runner PATH] [--repo PATH] [run_actions flags...]
#
#   --runner PATH  prebuilt run_actions.zip. Default: $RUN_ACTIONS, else build
#                  it with Bazel from the better_github checkout holding this
#                  script ($BAZEL, else bazel or bazelisk on PATH).
#   --repo PATH    datadog-agent checkout to run in. Default: $HOME/ws/datadog-agent
#   Any other flags (e.g. --serial, --noshow_errors_last) go to run_actions.
#
# To try it on another machine, copy this script and run_actions.zip there:
#   technical_linters.sh --runner ./run_actions.zip --repo ~/dd/datadog-agent
#
# Jobs left out, and why:
#   lint_licenses, check_modules_replace  need the go_deps / go_tools_deps
#                                         artifacts (.retrieve_linux_go_*deps)
#   validate_experiment_systemd_units,    only run on specific file changes
#   lint_releasenotes_rst,                (rules:), which needs the
#   lint_releasenotes_unique_ids,         changed-files selection we have
#   lint_rtloader                         not built yet

set -u

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo=$(cd "${script_dir}/../../" ; /bin/pwd)
runner_target="${RUN_ACTIONS:-@better_gitlab//run_actions:run_actions}"
passthrough=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --runner) runner="$2"; shift 2 ;;
    --runner=*) runner="${1#*=}"; shift ;;
    *) passthrough+=("$1"); shift ;;
  esac
done

bazel build "$runner_target"
runner=$(bazel info bazel-bin)/external/+http_archive+better_gitlab/run_actions/run_actions.pyz
# ls -l $(bazel info bazel-bin)/external/+http_archive+better_gitlab/run_actions
# echo info=$(bazel info bazel-bin)
echo runner=$runner

# Action names are the GitLab job names. validate_modules had two script
# lines, so it becomes two actions.
python3 "${runner}" "${passthrough[@]+"${passthrough[@]}"}" --ddci \
  lint_codeowners='dda inv -- -e github.lint-codeowner' \
  lint_components='dda inv -- -e lint-components lint-fxutil-oneshot-test' \
  lint_copyrights='dda inv -- -e linter.copyrights' \
  lint_docs_links='dda inv -- -e linter.docs-links' \
  lint_filename='dda inv -- -e linter.filenames' \
  lint_gopls_plugin='dda inv -- -e claude.set-buildtags --check' \
  lint_python='dda inv -- -e linter.python --show-versions' \
  lint_rust_licenses='dda inv -- -e lint-rust-licenses' \
  lint_shell='shellcheck --severity=info -e SC2059 -e SC2028 --shell=bash ./cmd/**/*.sh ./omnibus/package-scripts/*/*' \
  lint_shell_version='shellcheck --version' \
  lint_update_go='dda inv -- -e linter.update-go' \
  validate_modules='dda inv -- -e modules.validate' \
  validate_modules_used_by_otel='dda inv -- -e modules.validate-used-by-otel'
