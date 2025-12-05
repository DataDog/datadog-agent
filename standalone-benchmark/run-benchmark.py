import datetime
import os
import re
import statistics
import subprocess


NUM_RUNS = 80


def run_viper():
    os.environ['DD_CONF_NODETREEMODEL'] = 'no'
    p = subprocess.Popen('./standalone-benchmark/main', stdout=subprocess.PIPE)
    (stdout, _stderr) = p.communicate()
    stdout = stdout.decode('utf8')
    match = re.match(r'^allocated memory: (\d+)', stdout)
    if match:
        return int(match.group(1))
    raise RuntimeError('could not match output: %s' % stdout)


def run_ntm():
    os.environ['DD_CONF_NODETREEMODEL'] = 'enable'
    p = subprocess.Popen('./standalone-benchmark/main', stdout=subprocess.PIPE)
    (stdout, _stderr) = p.communicate()
    stdout = stdout.decode('utf8')
    match = re.match(r'^allocated memory: (\d+)', stdout)
    if match:
        return int(match.group(1))
    raise RuntimeError('could not match output: %s' % stdout)


def measure_viper():
    vals = []
    times = []
    for n in range(NUM_RUNS):
        before = datetime.datetime.now()
        v = run_viper()
        delta = datetime.datetime.now() - before
        vals.append(v)
        times.append(delta.total_seconds())
    print('Viper:')
    show_stats(vals, times)


def measure_ntm():
    vals = []
    times = []
    for n in range(NUM_RUNS):
        before = datetime.datetime.now()
        v = run_ntm()
        delta = datetime.datetime.now() - before
        vals.append(v)
        times.append(delta.total_seconds())
    print('NTM:')
    show_stats(vals, times)


def show_stats(vals, times):
    print(f' Mem  Mean:  {int(statistics.mean(vals)):_}')
    print(f' Mem  Stdev: {int(statistics.stdev(vals)):_}')
    print(f' Mem  Min:   {min(vals):_}')
    print(f' Mem  Max:   {max(vals):_}')
    print(f' Time Mean:  {statistics.mean(times):_}')
    print(f' Time Stdev: {statistics.stdev(times):_}')
    print(f' Time Min:   {min(times):_}')
    print(f' Time Max:   {max(times):_}')


def main():
    print('benchmarking, number of runs: %s' % NUM_RUNS)
    measure_viper()
    measure_ntm()


if __name__ == '__main__':
    main()