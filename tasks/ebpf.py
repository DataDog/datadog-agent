try:
    from termcolor import colored
except ImportError:
    colored = None
import os
import json
import contextlib
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
    "Total States"
]

def tabulate_stats(stats):
    table = list()
    for key, value in stats.items():
        row = list()
        row.append(key)
        row.append(value["stack_usage"])
        row.append(value["instruction_processed"])
        row.append(value["limit"])
        row.append(value["verification_time"])
        row.append(value["max_states_per_insn"])
        row.append(value["peak_states"])
        row.append(value["total_states"])
        table.append(row)

    return tabulate(table, headers=headers, tablefmt="grid")


def colored_diff(val1, val2):
    if val1 <= val2:
        return colored(val1-val2, "green")
    return colored(val1-val2, "red")

def stdout_or_file(filename=None):
    @contextlib.contextmanager
    def stdout():
        yield sys.stdout
    return open(filename, 'w') if filename else stdout()


@task
def print_verification_stats(ctx, skip_object_files=False, base=None, jsonfmt=False, out=None):
    sudo = "sudo" if not is_root() else ""
    if not skip_object_files:
        build_object_files(ctx)

    ctx.run("cd pkg/ebpf/verifier && go generate")
    ctx.run("go build -tags linux_bpf pkg/ebpf/verifier/calculator/main.go")
    res = ctx.run(f"{sudo} ./main pkg/ebpf/bytecode/build/co-re", hide=True)
    print(res.stderr)
    if res.exited == 0:
        verifier_stats = json.loads(res.stdout)
    else:
        return

    if base is None:
        with stdout_or_file(out) as f:
            if jsonfmt:
                print(json.dumps(verifier_stats, indent=4), file=f)
            else:
                print(tabulate_stats(verifier_stats), file=f)

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
        stat["stack_usage"] = colored_diff(value["stack_usage"], base_value["stack_usage"])
        stat["instruction_processed"] = colored_diff(value["instruction_processed"], base_value["instruction_processed"])
        stat["limit"] = colored_diff(value["limit"], base_value["limit"])
        stat["verification_time"] = colored_diff(value["verification_time"], base_value["verification_time"])
        stat["max_states_per_insn"] = colored_diff(value["max_states_per_insn"], base_value["max_states_per_insn"])
        stat["total_states"] = colored_diff(value["total_states"], base_value["total_states"])
        stat["peak_states"] = colored_diff(value["peak_states"], base_value["peak_states"])

        stats_diff[key] = stat


    with stdout_or_file(out) as f:
        if jsonfmt:
            print(json.dumps(stats_diff, indent=4), file=f)
        else:
            print(tabulate_stats(stats_diff), file=f)
