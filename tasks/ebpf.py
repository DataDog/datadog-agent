try:
    from termcolor import colored
except ImportError:
    colored = None
import contextlib
import json
import sys

from invoke import task

try:
    from tabulate import tabulate
except ImportError:
    tabulate = None

from .system_probe import build_object_files, is_root

headers = [
    "Program",
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


@task(
    help={
        "skip_object_files": "Do not build ebpf object files",
        "base": "JSON file holding verifier statistics to compare against",
        "jsonfmt": "Output in json format rather than tabulating",
        "out": "Output file to write results to. By default results are written to stdout",
    }
)
def print_verification_stats(ctx, skip_object_files=False, base=None, jsonfmt=False, out=None, debug_build=False):
    sudo = "sudo" if not is_root() else ""
    if not skip_object_files:
        build_object_files(ctx)

    # generate programs.go
    use_debug_build = "USE_DEBUG_BUILDS='true' " if debug_build else ""
    ctx.run(f"{use_debug_build}go generate pkg/ebpf/verifier/stats.go")

    ctx.run("cd pkg/ebpf/verifier && go generate")
    ctx.run("go build -tags linux_bpf pkg/ebpf/verifier/calculator/main.go")

    debug = "--debug" if debug_build else ""
    res = ctx.run(f"{sudo} ./main --directory pkg/ebpf/bytecode/build/co-re {debug}", hide=True)
    print(res.stderr, file=sys.stderr)
    if res.exited == 0:
        verifier_stats = json.loads(res.stdout)
    else:
        return

    if base is None:
        with stdout_or_file(out) as f:
            write_verifier_stats(verifier_stats, f, jsonfmt)
        return

    with open(base, 'r') as f:
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
