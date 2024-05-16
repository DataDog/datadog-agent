from __future__ import annotations

from tasks.libs.types.arch import get_arch

try:
    from termcolor import colored
except ImportError:
    colored = None
import collections
import contextlib
import json
import os
import re
import sys
from pathlib import Path
from typing import TYPE_CHECKING

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

if TYPE_CHECKING:
    from typing_extensions import TypedDict
else:
    TypedDict = dict

try:
    from tabulate import tabulate
except ImportError:
    tabulate = None

from .system_probe import build_cws_object_files, build_object_files, get_ebpf_build_dir, is_root

VERIFIER_DATA_DIR = Path("ebpf-calculator")
LOGS_DIR = VERIFIER_DATA_DIR / "logs"
VERIFIER_STATS = VERIFIER_DATA_DIR / "verifier_stats.json"
COMPLEXITY_DATA_DIR = VERIFIER_DATA_DIR / "complexity-data"

headers = [
    "Filename/Program",
    "Stack Usage",
    "Instructions Processed",
    "Instructions Processed limit",
    "Max States per Instruction",
    "Peak States",
    "Total States",
]

verifier_stat_json_keys = [
    "stack_usage",
    "instruction_processed",
    "limit",
    "max_states_per_insn",
    "peak_states",
    "total_states",
]

skip_stat_keys = ["Complexity", "verification_time"]


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

    return open(filename, "w") if filename else stdout()


def write_verifier_stats(verifier_stats, f, jsonfmt):
    if jsonfmt:
        print(json.dumps(verifier_stats, indent=4), file=f)
    else:
        print(tabulate_stats(verifier_stats), file=f)


# the go program return stats in the form {func_name: {stat_name: {Value: X}}}.
# convert this to {func_name: {stat_name: X}}
def format_verifier_stats(verifier_stats):
    filtered = dict()
    for func in verifier_stats:
        filtered[func] = dict()
        for stat in verifier_stats[func]:
            if stat not in skip_stat_keys:
                filtered[func][stat] = verifier_stats[func][stat]["Value"]
    return filtered


@task(
    help={
        "skip_object_files": "Do not build ebpf object files",
        "debug_build": "Collect verification statistics for debug builds",
        "filter_file": "List of files to load ebpf program from, specified without extension. By default we load everything",
        "grep": "Regex to filter program statistics",
        "line_complexity": "Generate per-line complexity data",
        "save_verifier_logs": "Save verifier logs to disk for debugging purposes",
    },
    iterable=["filter_file", "grep"],
)
def collect_verification_stats(
    ctx,
    skip_object_files=False,
    debug_build=False,
    filter_file: list[str] = None,  # type: ignore
    grep: list[str] = None,  # type: ignore
    line_complexity=False,
    save_verifier_logs=False,
):
    sudo = "sudo -E" if not is_root() else ""
    if not skip_object_files:
        build_object_files(ctx)
        build_cws_object_files(ctx)

    ctx.run("go build -tags linux_bpf pkg/ebpf/verifier/calculator/main.go")

    arch = get_arch("local")
    env = {"DD_SYSTEM_PROBE_BPF_DIR": f"./{get_ebpf_build_dir(arch)}"}

    # ensure all files are object files
    for f in filter_file or []:
        _, ext = os.path.splitext(f)
        if ext != ".o":
            raise Exit(f"File {f} does not have the valid '.o' extension")

    args = (
        [
            "-debug" if debug_build else "",
            "-summary-output",
            os.fspath(VERIFIER_STATS),
        ]
        + [f"-filter-file {f}" for f in filter_file or []]
        + [f"-filter-prog {p}" for p in grep or []]
    )

    if save_verifier_logs:
        LOGS_DIR.mkdir(exist_ok=True, parents=True)
        args += ["-verifier-logs", os.fspath(LOGS_DIR)]

    if line_complexity:
        COMPLEXITY_DATA_DIR.mkdir(exist_ok=True, parents=True)
        args += [
            "-line-complexity",
            "-complexity-data-dir",
            os.fspath(COMPLEXITY_DATA_DIR),
        ]

    ctx.run(f"{sudo} ./main {' '.join(args)}", env=env)

    # Ensure permissions are correct
    ctx.run(f"{sudo} chmod a+wr -R {VERIFIER_DATA_DIR}")
    ctx.run(f"{sudo} find {VERIFIER_DATA_DIR} -type d -exec chmod a+xr {{}} +")

    with open(VERIFIER_STATS, "r+") as f:
        verifier_stats = json.load(f)
        cleaned_up = format_verifier_stats(verifier_stats)
        f.seek(0)
        json.dump(cleaned_up, f, indent=4)
        f.truncate()


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

    with open(data) as f:
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
    print("  · A code [i|p|t] where:")
    print("    - i is the number of eBPF instructions generated by this source line")
    print(
        "    - p is the maximum number of passes the verifier made over assembly instructions generated by this source line"
    )
    print("    - t is the total number of instructions processed by the verifier for this source line")
    print("  · The line number, followed by the source code line, colored with the following key ")
    print("    based on the total number of instructions processed by the verifier for this line:")
    print(colored("    - Green: 0-5 instructions processed", "green"))
    print(colored("    - Yellow: 6-20 instructions processed", "yellow"))
    print(colored("    - Red: 21-50 instructions processed", "red"))
    print(colored("    - Magenta: 51+ instructions processed", "magenta"))
    print("\n\n")


class ComplexitySourceLine(TypedDict):
    num_instructions: int  # noqa: F841
    max_passes: int  # noqa: F841
    total_instructions_processed: int  # noqa: F841
    assembly_insns: list[int]  # noqa: F841


class ComplexityRegisterState(TypedDict):
    live: str  # noqa: F841
    type: str  # noqa: F841
    value: str  # noqa: F841
    register: int  # noqa: F841


class ComplexityAssemblyInsn(TypedDict):
    code: str  # noqa: F841
    index: int  # noqa: F841
    times_processed: int  # noqa: F841
    register_state: dict[str, ComplexityRegisterState]  # noqa: F841
    register_state_raw: str  # noqa: F841


class ComplexityData(TypedDict):
    source_map: dict[str, ComplexitySourceLine]  # noqa: F841
    insn_map: dict[str, ComplexityAssemblyInsn]  # noqa: F841


def get_total_complexity_stats_len(compinfo_widths: tuple[int, int, int]):
    return sum(compinfo_widths) + 7  # 7 = 2 brackets + 2 pipes + 3 letters


def source_line_to_str(
    lineno: int,
    line: str,
    compl: ComplexitySourceLine | None,
    compinfo_widths: tuple[int, int, int],
):
    color = None
    line = line.rstrip("\n")

    compinfo_len = get_total_complexity_stats_len(compinfo_widths)
    if compl is not None:
        insn = compl["num_instructions"]
        passes = compl["max_passes"]
        total = compl["total_instructions_processed"]
        compinfo = f"[{insn:{compinfo_widths[0]}d}i|{passes:{compinfo_widths[1]}d}p|{total:{compinfo_widths[2]}d}t]"

        if total <= 5:
            color = "green"
        elif total <= 20:
            color = "yellow"
        elif total <= 50:
            color = "red"
        else:
            color = "magenta"
    else:
        compinfo = " " * compinfo_len

    if colored is None:
        return f"{lineno:4d} | {compinfo} | {line}"
    else:
        return f"{lineno:4d} | {compinfo} | {colored(line, color)}"


def assembly_line_to_str(asm_line: ComplexityAssemblyInsn, compinfo_width: tuple[int, int, int]):
    asm_code = asm_line["code"]
    processed = asm_line["times_processed"]
    asm_idx = asm_line["index"]

    return f"{asm_idx:4d} | [{' '*compinfo_width[0]} |{processed:{compinfo_width[1]}d}p|{' '*compinfo_width[2]} ] | {asm_code}"


def register_state_to_str(reg: ComplexityRegisterState, compinfo_widths: tuple[int, int, int]):
    reg_liveness = f" [{reg['live']}]" if reg["live"] != "" else ""
    compinfo_len = get_total_complexity_stats_len(compinfo_widths)
    total_indent = 4 + 3 + compinfo_len  # Line number, pipe, complexity info
    return f"{' ' * total_indent} |    R{reg['register']} ({reg['type']}){reg_liveness}: {reg['value']}"


def get_complexity_for_function(object_file: str, function: str, debug=False) -> ComplexityData:
    if debug:
        object_file += "_debug"

    function = function.replace('/', '__')
    func_name = f"{object_file}/{function}"
    complexity_data_file = COMPLEXITY_DATA_DIR / f"{func_name}.json"

    if not os.path.exists(complexity_data_file):
        # Fall back to use section name
        print(
            f"Complexity data for function {func_name} not found at {complexity_data_file}, trying to find it as section..."
        )

        with open(COMPLEXITY_DATA_DIR / object_file / "mappings.json") as f:
            mappings = json.load(f)
        if func_name not in mappings:
            raise Exit(f"Cannot find complexity data for {func_name}, neither as function nor section name")

        funcs = mappings[function]
        if len(funcs) > 1:
            raise Exit(
                f"Multiple functions corresponding to section {func_name}: {funcs}. Please choose only one of them"
            )

        function = funcs[0]
        func_name = f"{object_file}/{function}"
        complexity_data_file = COMPLEXITY_DATA_DIR / f"{func_name}.json"

    with open(complexity_data_file) as f:
        return json.load(f)


@task(
    help={
        "object_file": "The program to analyze",
        "function": "The function to analyze",
        "debug": "Use debug builds",
        "show_assembly": "Show the assembly code for each line",
        "show_register_state": "Show the register state after each instruction",
        "assembly_instruction_limit": "Limit the number of assembly instructions to show",
        "show_raw_register_state": "Show the raw register state from the verifier after each instruction",
    }
)
def annotate_complexity(
    _: Context,
    object_file: str,
    function: str,
    debug=False,
    show_assembly=False,
    show_register_state=False,
    assembly_instruction_limit=20,
    show_raw_register_state=False,
):
    """Show source code with annotated complexity information for the given program and function"""
    complexity_data = get_complexity_for_function(object_file, function, debug)
    all_files = {x.split(":")[0] for x in complexity_data["source_map"].keys()}

    if colored is None:
        raise Exit("termcolor is required to print colored output")

    if show_register_state and not show_assembly:
        raise Exit("Register state can only be shown when assembly is also shown")

    print_complexity_legend()

    # Find out the width required to print the complexity stats
    max_insn = max_passes = max_total = 0
    for compl in complexity_data["source_map"].values():
        max_insn = max(max_insn, compl["num_instructions"])
        max_passes = max(max_passes, compl["max_passes"])
        max_total = max(max_total, compl["total_instructions_processed"])

    compinfo_widths = (len(str(max_insn)), len(str(max_passes)), len(str(max_total)))

    for f in sorted(all_files):
        if not os.path.exists(f):
            print(f"File {f} not found")
            continue

        print(colored(f"\n\n=== Source file: {f} ===", attrs=["bold"]))
        with open(f) as src:
            buffer = collections.deque(maxlen=10)  # Only print 10 lines of context if we have no line information
            for lineno, line in enumerate(src):
                lineno += 1
                lineid = f"{f}:{lineno}"
                compl = complexity_data["source_map"].get(lineid)
                statusline = source_line_to_str(lineno, line, compl, compinfo_widths)
                buffer.append(statusline)

                if compl is not None:
                    # We have complexity information for this line, print everything that we had in buffer
                    for l in buffer:
                        print(l)
                    buffer.clear()

                    if show_assembly:
                        # Print the assembly code for this line
                        asm_insn_indexes = sorted(compl["assembly_insns"])

                        if assembly_instruction_limit > 0:
                            asm_insn_indexes = asm_insn_indexes[:assembly_instruction_limit]

                        for asm_idx in asm_insn_indexes:
                            asm_line = complexity_data["insn_map"][str(asm_idx)]
                            asm_code = asm_line["code"]
                            print(
                                colored(
                                    assembly_line_to_str(asm_line, compinfo_widths),
                                    attrs=["dark"],
                                )
                            )

                            if show_register_state:
                                # Get all the registers that were used in this instruction
                                registers = re.findall(r"r\d+", asm_code)

                                # The register state after this instruction is found in the next instruction
                                # Also, JSON map indices are strings so we need to convert the asm_idx to a string
                                next_insn = str(asm_idx + 1)
                                if next_insn in complexity_data["insn_map"]:
                                    next_insn_data = complexity_data["insn_map"][next_insn]
                                    reg_state = next_insn_data["register_state"]

                                    if show_raw_register_state:
                                        total_indent = 4 + 3 + get_total_complexity_stats_len(compinfo_widths)
                                        raw_state = next_insn_data['register_state_raw'].split(':', 1)[1].strip()
                                        print(f"{' ' * total_indent} | {colored(raw_state, 'blue', attrs=['dark'])}")

                                    for reg in registers:
                                        reg_idx = reg[1:]  # Remove the 'r' prefix
                                        if reg_idx in reg_state:
                                            reg_data = reg_state[reg_idx]
                                            reg_info = register_state_to_str(reg_data, compinfo_widths)
                                            print(
                                                colored(
                                                    reg_info,
                                                    "blue",
                                                    attrs=["dark"],
                                                )
                                            )

                elif len(buffer) == 9:
                    # Print the last lines if we have no line information
                    for l in buffer:
                        print(l)

    print_complexity_legend()

    # Print the verification stats for this program now
    with open(VERIFIER_STATS) as f:
        verifier_stats = json.load(f)

    func_name = f"{object_file}/{function.replace('/', '__')}"
    if func_name not in verifier_stats:
        raise Exit(f"Verification stats for {func_name} not found in {VERIFIER_STATS}")

    print(colored("\n\n=== Verification stats ===", attrs=["bold"]))
    for key, value in verifier_stats[func_name].items():
        print(f"· {key}: {value}")


@task
def show_top_complexity_lines(
    _: Context,
    program: str,
    function: str,
    n=10,
    debug=False,
):
    """Show the lines with the most complexity for the given program and function"""
    complexity_data = get_complexity_for_function(program, function, debug)
    top_complexity_lines = sorted(
        complexity_data["source_map"].items(),
        key=lambda x: x[1]["total_instructions_processed"],
        reverse=True,
    )[:n]

    print_complexity_legend()
    compinfo_widths = None

    for lineid, compl in top_complexity_lines:
        f, lineno = lineid.split(":")
        if compinfo_widths is None:
            # Find out the widths, the first has the largest values so we go with that
            # Set a minimum just in case
            compinfo_widths = (
                max(2, len(str(compl["num_instructions"]))),
                max(2, len(str(compl["max_passes"]))),
                max(2, len(str(compl["total_instructions_processed"]))),
            )

        with open(f) as src:
            line = next(l for i, l in enumerate(src) if i + 1 == int(lineno))
            print(lineid)
            print(source_line_to_str(int(lineno), line, compl, compinfo_widths))
            print()
