try:
    from termcolor import colored
except ImportError:
    colored = None
import collections
import contextlib
import json
import os
import sys
from pathlib import Path
from typing import List

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

try:
    from tabulate import tabulate
except ImportError:
    tabulate = None

from .system_probe import build_cws_object_files, build_object_files, is_root

VERIFIER_DATA_DIR = Path("ebpf-calculator")
LOGS_DIR = VERIFIER_DATA_DIR / "logs"
VERIFIER_STATS = VERIFIER_DATA_DIR / "verifier_stats.json"
COMPLEXITY_DATA_DIR = VERIFIER_DATA_DIR / "complexity-data"

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

    if tabulate is None:
        raise Exit("tabulate is required to print verification stats")

    return tabulate(table, headers=headers, tablefmt="github")


def colored_diff(val1, val2):
    if colored is None:
        return val1 - val2

    if val1 <= val2:
        return colored(val1 - val2, "green")
    return colored(val1 - val2, "red")


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
            if stat != "Complexity":
                cleaned[func][stat] = verifier_stats[func][stat]["Value"]
    return cleaned


@task(
    help={
        "skip_object_files": "Do not build ebpf object files",
        "debug_build": "Collect verification statistics for debug builds",
        "filter_file": "List of files to load ebpf program from, specified without extension. By default we load everything",
        "grep": "Regex to filter program statistics",
        "line_complexity": "Generate per-line complexity data",
        "save_verifier_logs": "Save verifier logs to disk for debugging purposes",
    },
    iterable=['filter_file', 'grep'],
)
def collect_verification_stats(
    ctx,
    skip_object_files=False,
    debug_build=False,
    filter_file: List[str] = None,  # type: ignore
    grep: List[str] = None,  # type: ignore
    line_complexity=False,
    save_verifier_logs=False,
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
            "-summary-output",
            os.fspath(VERIFIER_STATS),
        ]
        + [f"-filter-file {f}" for f in filter_file]
        + [f"-filter-prog {p}" for p in grep]
    )

    if save_verifier_logs:
        LOGS_DIR.mkdir(exist_ok=True, parents=True)
        args += ["-verifier-logs", os.fspath(LOGS_DIR)]

    if line_complexity:
        COMPLEXITY_DATA_DIR.mkdir(exist_ok=True, parents=True)
        args += ["-line-complexity", "-complexity-data-dir", os.fspath(COMPLEXITY_DATA_DIR)]

    ctx.run(f"{sudo} ./main {' '.join(args)}", env=env)

    with open(VERIFIER_STATS, 'r') as f:
        verifier_stats = json.load(f)

    cleaned_up = cleanup_verifier_stats(verifier_stats)
    with open(VERIFIER_STATS, 'w') as f:
        json.dump(cleaned_up, f, indent=4)


@task(
    help={
        "base": "JSON file holding verifier statistics to compare against",
        "jsonfmt": "Output in json format rather than tabulating",
        "data": "JSON file witht the verifier statistics",
        "out": "Output file to write results to. By default results are written to stdout",
    }
)
def print_verification_stats(
    ctx,
    data=VERIFIER_STATS,
    base=None,
    jsonfmt=False,
    out=None,
):
    if not os.path.exists(data):
        print("[!] No verifier stats found, regenerating them...")
        collect_verification_stats(ctx)

    with open(data, 'r') as f:
        verifier_stats = json.load(f)

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


def print_complexity_legend():
    if colored is None:
        raise Exit("termcolor is required to print colored output")

    print("\n\n")
    print("## Complexity legend ##")
    print("For each source line the verifier processed, we output:")
    print("  · A code [Ii|Pp|Tt] where:")
    print("    - Ii is the number of eBPF instructions generated by this source line")
    print("    - Pp is the number of passes the verifier made over assembly generated by this source line")
    print("    - Tt is the total number of instructions processed by the verifier for this source line")
    print("  · The line number, followed by the source code line, colored with the following key ")
    print("    based on the total number of instructions processed by the verifier for this line:")
    print(colored("    - Green: 0-5 instructions processed", "green"))
    print(colored("    - Yellow: 6-20 instructions processed", "yellow"))
    print(colored("    - Red: 21-50 instructions processed", "red"))
    print(colored("    - Magenta: 51+ instructions processed", "magenta"))
    print("\n\n")


@task
def annotate_complexity(_: Context, program: str, function: str, debug=False):
    if debug:
        program += "_debug"

    func_name = f"{program}/{function.replace('/', '__')}"
    complexity_data_file = COMPLEXITY_DATA_DIR / f"{func_name}.json"

    if not os.path.exists(complexity_data_file):
        raise Exit(f"Complexity data for {func_name} not found at {complexity_data_file}")

    with open(complexity_data_file, 'r') as f:
        complexity_data = json.load(f)
    all_files = {x.split(':')[0] for x in complexity_data["source_map"].keys()}

    if colored is None:
        raise Exit("termcolor is required to print colored output")

    print_complexity_legend()

    for f in all_files:
        if not os.path.exists(f):
            print(f"File {f} not found")
            continue

        print(f"\n\n=== Source file: {f} ===")
        with open(f, 'r') as src:
            buffer = collections.deque(maxlen=10)  # Only print 10 lines of context if we have no line information
            for lineno, line in enumerate(src):
                lineno += 1
                lineid = f"{f}:{lineno}"
                linecomp = complexity_data["source_map"].get(lineid)
                color = None
                line = line.rstrip('\n')

                if linecomp is not None:
                    ins = linecomp["num_instructions"]
                    passes = linecomp["max_passes"]
                    total = ins * passes
                    if total <= 5:
                        color = "green"
                    elif total <= 20:
                        color = "yellow"
                    elif total <= 50:
                        color = "red"
                    else:
                        color = "magenta"

                    compinfo = f"[{ins:2d}i|{passes:2d}p|{total:2d}t]"
                else:
                    compinfo = " " * 13

                statusline = f"{lineno:4d} | {compinfo} | {colored(line, color)}"
                buffer.append(statusline)

                if compinfo.strip() != "":
                    for l in buffer:
                        print(l)
                    buffer.clear()
                elif len(buffer) == 9:
                    # Print the last lines if we have no line information
                    for l in buffer:
                        print(l)

    print_complexity_legend()
