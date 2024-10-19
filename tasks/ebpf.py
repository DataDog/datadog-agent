from __future__ import annotations

import itertools
import shutil

from tasks.github_tasks import pr_commenter
from tasks.kmt import download_complexity_data
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.git import get_commit_sha, get_common_ancestor, get_current_branch
from tasks.libs.types.arch import Arch

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
    table = []
    for key, value in stats.items():
        row = [key]
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


class ComplexitySummaryEntry(TypedDict):
    stack_usage: int  # noqa: F841
    instruction_processed: int  # noqa: F841
    limit: int  # noqa: F841
    max_states_per_insn: int  # noqa: F841
    peak_states: int  # noqa: F841
    total_states: int  # noqa: F841


ComplexitySummary = dict[str, ComplexitySummaryEntry]


# the go program return stats in the form {func_name: {stat_name: {Value: X}}}.
# convert this to {func_name: {stat_name: X}}
def format_verifier_stats(verifier_stats) -> ComplexitySummary:
    filtered = {}
    for func in verifier_stats:
        filtered[func] = {}
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

    arch = Arch.local()
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

    with open(VERIFIER_STATS, "r+") as file:
        verifier_stats = json.load(file)
        cleaned_up = format_verifier_stats(verifier_stats)
        file.seek(0)
        json.dump(cleaned_up, file, indent=4)
        file.truncate()


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

    stats_diff = {}
    for key, value in verifier_stats.items():
        stat = {}
        if key not in base_verifier_stats:
            stats_diff[key] = value
            continue

        base_value = base_verifier_stats[key]
        for json_key in verifier_stat_json_keys:
            if jsonfmt:
                stat[json_key] = value[json_key] - base_value[json_key]
            else:
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
    register_state: dict[str, ComplexityRegisterState]  # noqa: F841  # Register state after the instruction is executed
    register_state_raw: str  # noqa: F841


class ComplexityData(TypedDict):
    source_map: dict[str, ComplexitySourceLine]  # noqa: F841
    insn_map: dict[str, ComplexityAssemblyInsn]  # noqa: F841


def get_total_complexity_stats_len(compinfo_widths: tuple[int, int, int]):
    return sum(compinfo_widths) + 7  # 7 = 2 brackets + 2 pipes + 3 letters


COMPLEXITY_THRESHOLD_LOW = 5
COMPLEXITY_THRESHOLD_MEDIUM = 20
COMPLEXITY_THRESHOLD_HIGH = 50


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

        if total <= COMPLEXITY_THRESHOLD_LOW:
            color = "green"
        elif total <= COMPLEXITY_THRESHOLD_MEDIUM:
            color = "yellow"
        elif total <= COMPLEXITY_THRESHOLD_HIGH:
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


def _get_sorted_list_of_files(complexity_data: ComplexityData):
    num_asm_insns = len(complexity_data["insn_map"])
    files_and_min_asm: dict[str, int] = collections.defaultdict(lambda: num_asm_insns)
    for lineid, compl in complexity_data["source_map"].items():
        file = lineid.split(":")[0]
        min_asm_for_line = min(compl["assembly_insns"], default=num_asm_insns)
        files_and_min_asm[file] = min(files_and_min_asm[file], min_asm_for_line)

    return [x[0] for x in sorted(files_and_min_asm.items(), key=lambda x: x[1])]


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
    all_files = _get_sorted_list_of_files(complexity_data)

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

    for f in all_files:
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
                    for lb in buffer:
                        print(lb)
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

                            # This is the register state after the statement is executed
                            reg_state = asm_line.get("register_state")
                            if show_register_state and reg_state is not None:
                                # Get all the registers that were used in this instruction
                                registers = re.findall(r"r\d+", asm_code)

                                if show_raw_register_state:
                                    total_indent = 4 + 3 + get_total_complexity_stats_len(compinfo_widths)
                                    raw_state = asm_line['register_state_raw'].split(':', 1)[1].strip()
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
                    for lb in buffer:
                        print(lb)

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
            line = next(line for i, line in enumerate(src) if i + 1 == int(lineno))
            print(lineid)
            print(source_line_to_str(int(lineno), line, compl, compinfo_widths))
            print()


@task
def generate_html_report(ctx: Context, dest_folder: str | Path):
    """Generate an HTML report with the complexity data"""
    try:
        from jinja2 import Environment, FileSystemLoader, select_autoescape
    except ImportError as e:
        raise Exit("jinja2 is required to generate the HTML report") from e

    if not VERIFIER_STATS.exists() or not COMPLEXITY_DATA_DIR.exists():
        print("[!] No verifier stats found, regenerating them...")
        collect_verification_stats(ctx, line_complexity=True)

    dest_folder = Path(dest_folder)
    dest_folder.mkdir(exist_ok=True, parents=True)

    with open(VERIFIER_STATS) as f:
        verifier_stats = json.load(f)

    stats_by_object_and_program = collections.defaultdict(dict)

    for prog, stats in verifier_stats.items():
        object_file, function = prog.split("/")
        stats_by_object_and_program[object_file][function] = stats

    env = Environment(
        loader=FileSystemLoader(Path(__file__).parent / "ebpf_verifier/html/templates"),
        autoescape=select_autoescape(),
        trim_blocks=True,
    )

    template = env.get_template("index.html.j2")
    render = template.render(
        title=f"eBPF complexity report - {get_current_branch(ctx)} - {get_commit_sha(ctx, short=True)}",
        verifier_stats=stats_by_object_and_program,
    )

    index_file = dest_folder / "index.html"
    index_file.write_text(render)

    for file in COMPLEXITY_DATA_DIR.glob("**/*.json"):
        object_file = file.parent.name
        function = file.stem

        if function == "mappings":
            continue  # Ignore the mappings file

        print(f"Generating report for {object_file}/{function}...")

        with open(file) as f:
            complexity_data: ComplexityData = json.load(f)

        if "source_map" not in complexity_data:
            print("Invalid complexity data file", file)
            continue

        # Define the complexity level for all assembly instructions
        for insn in complexity_data["insn_map"].values():
            if insn['times_processed'] <= COMPLEXITY_THRESHOLD_LOW:
                level = 'low'
            elif insn['times_processed'] <= COMPLEXITY_THRESHOLD_MEDIUM:
                level = 'medium'
            elif insn['times_processed'] <= COMPLEXITY_THRESHOLD_HIGH:
                level = 'high'
            else:
                level = 'extreme'
            insn['complexity_level'] = level  # type: ignore

        all_files = _get_sorted_list_of_files(complexity_data)
        file_contents = {}
        for f in all_files:
            if not os.path.exists(f):
                print(f"File {f} not found")
                continue

            with open(f) as src:
                file_contents[f] = []
                for lineno, line in enumerate(src.read().splitlines()):
                    lineid = f"{f}:{lineno + 1}"
                    compl = complexity_data["source_map"].get(lineid)
                    linedata = {"line": line, "complexity": compl}
                    if compl is not None:
                        if compl['num_instructions'] <= COMPLEXITY_THRESHOLD_LOW:
                            linedata['complexity_level'] = 'low'
                        elif compl['num_instructions'] <= COMPLEXITY_THRESHOLD_MEDIUM:
                            linedata['complexity_level'] = 'medium'
                        elif compl['num_instructions'] <= COMPLEXITY_THRESHOLD_HIGH:
                            linedata['complexity_level'] = 'high'
                        else:
                            linedata['complexity_level'] = 'extreme'
                    else:
                        linedata['complexity_level'] = 'none'

                    file_contents[f].append(linedata)

        template = env.get_template("program.html.j2")
        render = template.render(
            title=f"{object_file}/{function} complexity analysis",
            object_file=object_file,
            function=function,
            complexity_data=complexity_data,
            file_contents=file_contents,
        )
        object_folder = dest_folder / object_file
        object_folder.mkdir(exist_ok=True, parents=True)
        complexity_file = object_folder / f"{function}.html"
        complexity_file.write_text(render)

    # Copy all static files
    static_files = Path(__file__).parent / "ebpf_verifier/html/static"
    for file in static_files.glob("*"):
        print(f"Copying static {file} to {dest_folder}")
        shutil.copy(file, dest_folder)


@task(
    help={
        "skip_github_comment": "Do not comment on the PR with the complexity summary",
        "branch_name": "Branch name to use for the complexity data. By default, the current branch is used",
        "base_branch": "Base branch to compare against. If not provided, we will try to find a PR for the current branch, and use the base branch from there. If that fails, the main branch will be used",
    }
)
def generate_complexity_summary_for_pr(
    ctx: Context, skip_github_comment=False, branch_name: str | None = None, base_branch: str | None = None
):
    """Task meant to run in CI. Generates a summary of the complexity data for the current PR"""
    if tabulate is None:
        raise Exit("tabulate is required to print the complexity summary")

    if branch_name is None:
        branch_name = os.getenv("CI_COMMIT_REF_NAME") or get_current_branch(ctx)
        commit_sha = os.getenv("CI_COMMIT_SHA") or get_commit_sha(ctx)
    else:
        commit_sha = get_commit_sha(ctx, branch_name)

    pr_comment_head = 'eBPF complexity changes'
    github = GithubAPI()
    prs = list(github.get_pr_for_branch(branch_name))
    has_prs = len(prs) > 0

    if base_branch is None:
        if len(prs) == 0:
            print(f"Warning: No PR found for branch {branch_name}, using main branch as base")
            base_branch = "main"
        elif len(prs) > 1:
            print(f"Warning: Multiple PRs found for branch {branch_name}, using main branch as base")
            base_branch = "main"
        else:
            base_branch = prs[0].base.ref
            print(f"Found PR {prs[0].number} for this branch, using base branch {base_branch}")

    def _try_delete_github_comment(msg: str):
        if skip_github_comment or not has_prs:
            print(f"{msg}: exiting ({skip_github_comment=}, {has_prs=})")
        else:
            print(f"{msg}: removing GitHub comment in PR")
            pr_commenter(ctx, pr_comment_head, delete=True, force_delete=True)

    # First, ensure we have the complexity data for our current branch
    current_branch_artifacts_path: Path | str | None = os.getenv("DD_AGENT_TESTING_DIR")
    if current_branch_artifacts_path is None:
        raise Exit("DD_AGENT_TESTING_DIR is not set, cannot find the complexity data")

    current_branch_artifacts_path = Path(current_branch_artifacts_path)
    complexity_files = list(current_branch_artifacts_path.glob("verifier-complexity-*.tar.gz"))
    if len(complexity_files) == 0:
        _try_delete_github_comment(
            f"No complexity data found for the current branch at {current_branch_artifacts_path}"
        )
        return

    # We have files, now get files for the main branch
    common_ancestor = get_common_ancestor(ctx, commit_sha, f"origin/{base_branch}")
    main_branch_complexity_path = Path("/tmp/verifier-complexity-main")
    print(f"Downloading complexity data for {base_branch} branch (commit {common_ancestor})...")
    download_complexity_data(ctx, common_ancestor, main_branch_complexity_path)

    main_complexity_files = list(main_branch_complexity_path.glob("verifier-complexity-*"))
    if len(main_complexity_files) == 0:
        _try_delete_github_comment(f"No complexity data found for the main branch at {main_branch_complexity_path}")
        return

    # Uncompress all local complexity files, and store the results
    program_complexity = collections.defaultdict(list)  # Map each program to a list of
    for file in complexity_files:
        folder_name = file.name.replace('.tar.gz', '')
        target_dir = current_branch_artifacts_path / folder_name
        ctx.run(f"mkdir -p {target_dir}")
        ctx.run(f"tar -xzf {file} -C {target_dir}")

        main_data_path = main_branch_complexity_path / folder_name / "verifier_stats.json"
        if not main_data_path.exists():
            print(f"Error: Main branch data not found for {main_data_path}, skipping")
            continue

        branch_data_path = target_dir / "verifier_stats.json"
        if not branch_data_path.exists():
            print(f"Error: Branch data not found for {branch_data_path}, skipping")
            continue

        # Don't care about the splits at the last part of the name (security-agent, system-probe, etc)
        name_parts = folder_name.split('-', 4)
        if len(name_parts) != 5:
            print(f"Error: Invalid folder name {folder_name}, skipping")
            continue

        arch = name_parts[2]
        distro = name_parts[3]

        try:
            main_data = format_verifier_stats(json.loads(main_data_path.read_text()))
        except Exception as e:
            print(f"Error: Could not load data for {main_data_path}: {e}")
            continue

        try:
            branch_data = format_verifier_stats(json.loads(branch_data_path.read_text()))
        except Exception as e:
            print(f"Error: Could not load data for {branch_data_path}: {e}")
            continue

        for program, stats in branch_data.items():
            if program not in main_data:
                print(f"Warn: Program {program} not found in main data for {folder_name}, skipping")
                continue

            program_complexity[program].append(
                (
                    arch,
                    distro,
                    stats["instruction_processed"],
                    main_data[program]["instruction_processed"],
                    stats["limit"],
                )
            )

    if len(program_complexity) == 0:
        _try_delete_github_comment("No complexity data found, skipping report generation")
        return

    summarized_complexity_changes = []
    max_complexity_rel_change = -9e9
    max_complexity_abs_change = -9e9
    threshold_for_max_limit = 0.85
    programs_now_below_limit, programs_now_above_limit = 0, 0
    for program, entries in sorted(program_complexity.items(), key=lambda x: max(e[2] for e in x[1])):
        avg_new_complexity, avg_old_complexity = 0, 0
        highest_new_complexity, highest_old_complexity = 0, 0
        lowest_new_complexity, lowest_old_complexity = 1e9, 1e9  # instruction limit is always < 1e9
        highest_complexity_platform, lowest_complexity_platform = "", ""

        for arch, distro, new_complexity, old_complexity, limit in entries:
            avg_new_complexity += new_complexity
            avg_old_complexity += old_complexity

            if new_complexity > highest_new_complexity:
                highest_new_complexity = new_complexity
                highest_old_complexity = old_complexity
                highest_complexity_platform = f"{distro}/{arch}"

            if new_complexity < lowest_new_complexity:
                lowest_new_complexity = new_complexity
                lowest_old_complexity = old_complexity
                lowest_complexity_platform = f"{distro}/{arch}"

            new_threshold = new_complexity / limit
            old_threshold = old_complexity / limit

            if new_threshold < threshold_for_max_limit and old_threshold >= threshold_for_max_limit:
                programs_now_below_limit += 1
            elif new_threshold >= threshold_for_max_limit and old_threshold < threshold_for_max_limit:
                programs_now_above_limit += 1

            abs_change = new_complexity - old_complexity
            max_complexity_rel_change = max(max_complexity_rel_change, abs_change / old_complexity)
            max_complexity_abs_change = max(max_complexity_abs_change, abs_change)

        avg_new_complexity /= len(entries)
        avg_old_complexity /= len(entries)

        has_changes = (
            avg_new_complexity != avg_old_complexity
            or highest_new_complexity != highest_old_complexity
            or lowest_new_complexity != lowest_old_complexity
        )

        row = [
            program,
            _format_change(avg_new_complexity, avg_old_complexity),
            f"{highest_complexity_platform}: {_format_change(highest_new_complexity, highest_old_complexity)}",
            f"{lowest_complexity_platform}: {_format_change(lowest_new_complexity, lowest_old_complexity)}",
            has_changes,
        ]
        summarized_complexity_changes.append(row)

    headers = ["Program", "Avg. complexity", "Distro with highest complexity", "Distro with lowest complexity"]
    summarized_complexity_changes = sorted(summarized_complexity_changes, key=lambda x: x[0])

    if max_complexity_rel_change < 0 and max_complexity_abs_change < 0 and programs_now_above_limit == 0:
        state = "🎉 - improved"
    elif max_complexity_rel_change < 0.05 and max_complexity_abs_change < 100 and programs_now_above_limit == 0:
        state = "✅ - stable"
    elif max_complexity_rel_change < 0.15 or max_complexity_abs_change < 1000 and programs_now_above_limit == 0:
        state = "❔ - needs attention"
    else:
        state = "❗ - significant complexity increases"

    pipeline_id = os.getenv("CI_PIPELINE_ID")

    msg = f"### Summary result: {state}\n\n"
    msg += f"* Highest complexity change (%): {max_complexity_rel_change * 100:+.2f}%\n"
    msg += f"* Highest complexity change (abs.): {max_complexity_abs_change:+} instructions\n"
    msg += f"* Programs that were above the {threshold_for_max_limit * 100}% limit of instructions and are now below: {programs_now_below_limit}\n"
    msg += f"* Programs that were below the {threshold_for_max_limit * 100}% limit of instructions and are now above: {programs_now_above_limit}\n"
    msg += "\n\n"

    has_any_changes = False
    for group, rows in itertools.groupby(summarized_complexity_changes, key=lambda x: x[0].split("/")[0]):
        if not any(row[-1] for row in rows):
            continue

        rows = list(rows)  # Convert the iterator to a list, so we can iterate over it multiple times

        def _build_table(orig_rows):
            # Format rows to make it more compact, remove the changes marker and remove the object name
            changed_rows = [[row[0].split("/")[1]] + row[1:-1] for row in orig_rows]
            assert tabulate is not None  # For typing
            return tabulate(changed_rows, headers=headers, tablefmt="github")

        with_changes = [row for row in rows if row[-1]]
        without_changes = [row for row in rows if not row[-1]]

        msg += f"\n<details><summary>{group} details</summary>\n\n"
        msg += f"## {group} [programs with changes]\n\n"
        msg += _build_table(with_changes)
        msg += f"\n\n## {group} [programs without changes]\n\n"
        msg += _build_table(without_changes)
        msg += "\n\n</details>\n"
        has_any_changes = True

    curr_commit = get_commit_sha(ctx, short=False)
    msg += f"\n\nThis report was generated based on the complexity data for the current branch {branch_name} (pipeline [{pipeline_id}](https://gitlab.ddbuild.io/DataDog/datadog-agent/-/pipelines/{pipeline_id}), commit {curr_commit}) and the base branch {base_branch} (commit {common_ancestor}). Objects without changes are not reported. Contact [#ebpf-platform](https://dd.enterprise.slack.com/archives/C0424HA1SJK) if you have any questions/feedback."
    msg += "\n\nTable complexity legend: 🔵 - new; ⚪ - unchanged; 🟢 - reduced; 🔴 - increased"

    print(msg)

    if skip_github_comment or not has_prs:
        print("Skipping commenting on PR")
        return

    if not has_any_changes:
        print("No changes detected, deleting comment")
        pr_commenter(ctx, pr_comment_head, "", delete=True, force_delete=True)
        return

    pr_commenter(ctx, pr_comment_head, msg)
    print("Commented on PR with complexity summary")


def _format_change(new: float, old: float):
    change_abs = new - old

    if old == 0:
        change_rel_str = "[NEW]"
        emoji = "🔵"
    else:
        change_rel = change_abs / old
        change_rel_str = f"{change_rel:+.2%}"

        if change_rel == 0:
            emoji = "⚪"
        elif change_rel < 0:
            emoji = "🟢"
        else:
            emoji = "🔴"

    return f"{emoji} {new:.1f} ({change_abs:+.1f}, {change_rel_str})"
