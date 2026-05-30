"""
Finds the "common paths" of settings registered via BindEnvAndSetDefault
inside initCoreAgentFull that don't appear outside of it.

For each setting exclusive to initCoreAgentFull, finds the shortest path
prefix that has no outside settings under it, then deduplicates. This
compresses groups of related settings (e.g. 24 network_path.collector.*
settings) into a single prefix entry.

Output is written to paths.py in the same directory.
"""

import re
import glob
import os

SETUP_DIR = os.path.dirname(os.path.abspath(__file__))
COMMON_SETTINGS = os.path.join(SETUP_DIR, 'common_settings.go')
OUTPUT_FILE = os.path.join(SETUP_DIR, 'paths.py')

BIND_PATTERN = r'BindEnvAndSetDefault\(\s*"([^"]+)"'


def extract_function_body(content, func_name):
    """Return (inside, outside) text for a named function in content."""
    func_start = content.find(f'func {func_name}(')
    if func_start == -1:
        raise ValueError(f'Function {func_name} not found')
    brace_pos = content.find('{', func_start)
    depth = 0
    for i in range(brace_pos, len(content)):
        if content[i] == '{':
            depth += 1
        elif content[i] == '}':
            depth -= 1
            if depth == 0:
                func_end = i
                break
    inside = content[brace_pos:func_end + 1]
    outside = content[:func_start] + content[func_end + 1:]
    return inside, outside


def has_outside_with_prefix(prefix, outside_set):
    prefix_dot = prefix + '.'
    return any(o == prefix or o.startswith(prefix_dot) for o in outside_set)


def get_shortest_valid_prefix(setting, outside_set):
    parts = setting.split('.')
    for length in range(1, len(parts) + 1):
        prefix = '.'.join(parts[:length])
        if not has_outside_with_prefix(prefix, outside_set):
            return prefix
    return setting


def main():
    with open(COMMON_SETTINGS) as f:
        content = f.read()

    inside_func, outside_func = extract_function_body(content, 'initCoreAgentFull')

    inside_settings = set(re.findall(BIND_PATTERN, inside_func))
    outside_settings = set(re.findall(BIND_PATTERN, outside_func))

    for fpath in glob.glob(os.path.join(SETUP_DIR, '*.go')):
        if os.path.basename(fpath) == 'common_settings.go':
            continue
        with open(fpath) as f:
            outside_settings.update(re.findall(BIND_PATTERN, f.read()))

    only_inside = inside_settings - outside_settings

    prefixes = set()
    for setting in only_inside:
        prefixes.add(get_shortest_valid_prefix(setting, outside_settings))

    sorted_prefixes = sorted(prefixes)

    with open(OUTPUT_FILE, 'w') as f:
        f.write('paths = [\n')
        for p in sorted_prefixes:
            f.write(f'    "{p}",\n')
        f.write(']\n')

    print(f'Written {len(sorted_prefixes)} paths to {OUTPUT_FILE}')


if __name__ == '__main__':
    main()
