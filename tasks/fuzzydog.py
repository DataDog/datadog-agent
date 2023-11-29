"""
Helper for running fuzz targets in fuzzing CI.
"""
import os

from invoke import task


@task
def fuzzydog(ctx, version, duration="600"):
    """
    TODO
    """
    cwd = os.getcwd()
    for directory, funcs in search_fuzz_tests(cwd):
        with ctx.cd(directory):
            target_binary = remove_prefix(cwd, directory).replace(os.path.sep, "_")
            bld = f'go test -c -o ./{target_binary}'
            ctx.run(bld)
            fzz = f'fuzzydog fuzzer create datadog-agent --duration={duration} --version={version} --binary={target_binary} --type=go-native-fuzz --team=datadog-agent'
            ctx.run(fzz)
            ctx.run(f'rm ./{target_binary}')

def search_fuzz_tests(directory):
    """
    Yields (directory, [fuzz function name]) tuples.
    """
    for file in os.listdir(directory):
        path = os.path.join(directory, file)
        if os.path.isdir(path):
            for tuple in search_fuzz_tests(path):
                yield tuple
        else:
            if not file.endswith('_test.go'):
                continue
            with open(path) as f:
                funcs = []
                for line in f.readlines():
                    if line.startswith('func Fuzz'):
                        fuzzfunc = line[5 : line.find('(')]  # 5 is len('func ')
                        funcs.append(fuzzfunc)
                if funcs:
                    yield (directory, funcs)


def remove_prefix(prefix, directory):
    directory = os.path.normpath(directory)
    prefix = os.path.normpath(prefix)

    if directory.startswith(prefix):
        return directory[len(prefix)+1:]
    else:
        return directory
