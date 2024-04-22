try:
    from termcolor import colored
except ImportError:
    colored = None
import contextlib
import json
import os
import sys

from invoke import task
from invoke.exceptions import Exit

try:
    from tabulate import tabulate
except ImportError:
    tabulate = None

from .system_probe import build_cws_object_files, build_object_files, is_root

headers = [
    "Filename/Program",
    "Stack Usage",
    "Instructions Processed",
    "Instructions Processed limit",
    "Verification time",
    "Max States per Instruction",
    "Peak States",
    "Total States",
]

verifier_stat_json_keys = [
    "stack_usage",
    "instruction_processed",
    "limit",
    "verification_time",
    "max_states_per_insn",
    "peak_states",
    "total_states",
]


def tabulate_stats(stats):
    table = list()
    for key, value in stats.items():
        row = list()
        row.append(key)
        for json_key in verifier_stat_json_keys:
            row.append(value[json_key])
        table.append(row)

    return tabulate(table, headers=headers, tablefmt="github")


def colored_diff(val1, val2):
    try:
        if val1 <= val2:
            return colored(val1 - val2, "green")
        return colored(val1 - val2, "red")
    except TypeError:
        return val1 - val2


def stdout_or_file(filename=None):
    @contextlib.contextmanager
    def stdout():
        yield sys.stdout

    return open(filename, 'w') if filename else stdout()


def write_verifier_stats(verifier_stats, f, jsonfmt):
    if jsonfmt:
        print(json.dumps(verifier_stats, indent=4), file=f)
    else:
        print(tabulate_stats(verifier_stats), file=f)


# the go program return stats in the form {func_name: {stat_name: {Value: X}}}.
# convert this to {func_name: {stat_name: X}}
def cleanup_verifier_stats(verifier_stats):
    cleaned = dict()
    for func in verifier_stats:
        cleaned[func] = dict()
        for stat in verifier_stats[func]:
            cleaned[func][stat] = verifier_stats[func][stat]["Value"]
    return cleaned


@task(
    help={
        "skip_object_files": "Do not build ebpf object files",
        "base": "JSON file holding verifier statistics to compare against",
        "jsonfmt": "Output in json format rather than tabulating",
        "out": "Output file to write results to. By default results are written to stdout",
        "debug_build": "Collect verification statistics for debug builds",
        "filter_file": "List of files to load ebpf program from, specified without extension. By default we load everything",
        "grep": "Regex to filter program statistics",
    },
    iterable=['filter_file', 'grep'],
)
def print_verification_stats(
    ctx,
    skip_object_files=False,
    base=None,
    jsonfmt=False,
    out=None,
    debug_build=False,
    filter_file=None,
    grep=None,
):
    sudo = "sudo -E" if not is_root() else ""
    if not skip_object_files:
        build_object_files(ctx)
        build_cws_object_files(ctx)

    ctx.run("go build -tags linux_bpf pkg/ebpf/verifier/calculator/main.go")

    env = {"DD_SYSTEM_PROBE_BPF_DIR": "./pkg/ebpf/bytecode/build"}

    # ensure all files are object files
    for f in filter_file:
        _, ext = os.path.splitext(f)
        if ext != ".o":
            raise Exit(f"File {f} does not have the valid '.o' extension")

    args = (
        [
            "-debug" if debug_build else "",
        ]
        + [f"-filter-file {f}" for f in filter_file]
        + [f"-filter-prog {p}" for p in grep]
    )
    res = ctx.run(f"{sudo} ./main {' '.join(args)}", env=env, hide='out')
    if res.exited == 0:
        verifier_stats = cleanup_verifier_stats(json.loads(res.stdout))
    else:
        return

    if base is None:
        with stdout_or_file(out) as f:
            write_verifier_stats(verifier_stats, f, jsonfmt)
        return

    with open(base) as f:
        base_verifier_stats = json.load(f)

    stats_diff = dict()
    for key, value in verifier_stats.items():
        stat = dict()
        if key not in base_verifier_stats:
            stats_diff[key] = value
            continue

        base_value = base_verifier_stats[key]
        for json_key in verifier_stat_json_keys:
            stat[json_key] = colored_diff(value[json_key], base_value[json_key])

        stats_diff[key] = stat

    with stdout_or_file(out) as f:
        write_verifier_stats(stats_diff, f, jsonfmt)
