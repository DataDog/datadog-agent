#!/usr/bin/env python3
"""
Reconstruct the orchestrator check's lifecycle across DCA + runners,
correlating dispatches, schedules, run-starts, run-dones, cancels, and
rebalance-proposal presence/absence.
"""
import re, json, glob
from datetime import datetime, timezone
from pathlib import Path

REPRO_DIR = Path('/tmp/repro2')

def parse_ts(s):
    # "2026-05-18 20:54:09 UTC"
    return datetime.strptime(s.replace(' UTC', ''), '%Y-%m-%d %H:%M:%S').replace(tzinfo=timezone.utc)

events = []

# Parse DCA log
dca_path = REPRO_DIR / 'dca.log'
with open(dca_path) as f:
    log = f.read()
for line in log.split('\n'):
    if 'Found a better' in line and 'rebalanceUsingUtilization' in line:
        m = re.match(r'^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) UTC', line)
        if not m: continue
        ts = parse_ts(m.group(1))
        # Try to find the proposal JSON (may span multiple lines if pretty-printed, but here it's on one line)
        jm = re.search(r'Proposed distribution: (\{.*\})$', line)
        if not jm:
            continue
        try:
            data = json.loads(jm.group(1))
        except Exception:
            continue
        checks = data.get('Checks', {})
        runners = data.get('Runners', {})
        orch_keys = [k for k in checks if k.startswith('orchestrator')]
        orch_present = bool(orch_keys)
        orch_runner = ''
        orch_wn = 0
        if orch_keys:
            orch_runner = checks[orch_keys[0]]['Runner'].split('-')[-1]
            orch_wn = checks[orch_keys[0]]['WorkersNeeded']
        stddev_curr = re.search(r'StdDev of current distribution: ([\d.]+?)\.\s', line)
        stddev_prop = re.search(r'stdDev of proposed distribution: ([\d.]+?)\.\s', line)
        events.append({
            'ts': ts,
            'type': 'REBALANCE',
            'orch_present': orch_present,
            'orch_runner': orch_runner,
            'orch_wn': orch_wn,
            'total_checks_in_proposal': len(checks),
            'runners_used': {nm.split('-')[-1]: r['WorkersUsed'] for nm, r in runners.items()},
            'stddev_curr': float(stddev_curr.group(1)) if stddev_curr else 0,
            'stddev_prop': float(stddev_prop.group(1)) if stddev_prop else 0,
        })
    elif 'Dispatching configuration orchestrator' in line:
        m = re.match(r'^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) UTC', line)
        if not m: continue
        ts = parse_ts(m.group(1))
        runner_match = re.search(r'to node (\S+)', line)
        cfg_match = re.search(r'configuration (orchestrator:\S+) to', line)
        events.append({
            'ts': ts, 'type': 'DISPATCH',
            'cfg': cfg_match.group(1) if cfg_match else '?',
            'runner': runner_match.group(1).rstrip('"').split('-')[-1] if runner_match else '?',
        })

# Parse runner logs
for runner_log in sorted(glob.glob(str(REPRO_DIR / 'runner-*.log'))):
    nm = Path(runner_log).stem.replace('runner-', '').split('-')[-1]
    with open(runner_log) as f:
        for line in f:
            ts_m = re.match(r'^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) UTC', line)
            if not ts_m: continue
            ts = parse_ts(ts_m.group(1))
            if 'Scheduling check orchestrator' in line:
                cfg_match = re.search(r'(orchestrator:\S+)', line)
                events.append({
                    'ts': ts, 'type': 'SCHEDULE', 'runner': nm,
                    'cfg': cfg_match.group(1) if cfg_match else '?',
                })
            elif 'check:orchestrator | Running check' in line:
                events.append({
                    'ts': ts, 'type': 'RUN_START', 'runner': nm,
                })
            elif 'check:orchestrator | Done running check' in line:
                events.append({
                    'ts': ts, 'type': 'RUN_DONE', 'runner': nm,
                })
            elif "informers used by the check 'orchestrator" in line:
                cfg_match = re.search(r"'(orchestrator:\S+)'", line)
                events.append({
                    'ts': ts, 'type': 'CANCEL', 'runner': nm,
                    'cfg': cfg_match.group(1) if cfg_match else '?',
                })
            elif 'Unscheduling check orchestrator' in line:
                cfg_match = re.search(r'(orchestrator:\S+)', line)
                events.append({
                    'ts': ts, 'type': 'UNSCHEDULE', 'runner': nm,
                    'cfg': cfg_match.group(1) if cfg_match else '?',
                })

events.sort(key=lambda e: e['ts'])

# Print timeline
print(f"{'time (UTC)':<22} {'event':<12} {'runner':<8} {'detail'}")
print('-' * 100)
prev_ts = None
for ev in events:
    delta = ''
    if prev_ts:
        d = (ev['ts'] - prev_ts).total_seconds()
        if d < 60: delta = f"+{d:.0f}s"
        else: delta = f"+{d/60:.1f}m"
    if ev['type'] == 'REBALANCE':
        if ev['orch_present']:
            detail = f"ORCH ON {ev['orch_runner']:>5s} wn={ev['orch_wn']:.2f} | {ev['total_checks_in_proposal']} checks | runners={ev['runners_used']}"
        else:
            detail = f"ORCH ABSENT | {ev['total_checks_in_proposal']} checks | runners={ev['runners_used']}"
        print(f"{ev['ts'].strftime('%H:%M:%S'):<22} {'REBALANCE':<12} {'-':<8} {delta:<6} {detail}")
    elif ev['type'] == 'DISPATCH':
        print(f"{ev['ts'].strftime('%H:%M:%S'):<22} {'DISPATCH':<12} {ev['runner']:<8} {delta:<6} {ev['cfg']}")
    elif ev['type'] == 'SCHEDULE':
        print(f"{ev['ts'].strftime('%H:%M:%S'):<22} {'SCHEDULE':<12} {ev['runner']:<8} {delta:<6} {ev['cfg']}")
    elif ev['type'] == 'RUN_START':
        print(f"{ev['ts'].strftime('%H:%M:%S'):<22} {'RUN_START':<12} {ev['runner']:<8} {delta}")
    elif ev['type'] == 'RUN_DONE':
        print(f"{ev['ts'].strftime('%H:%M:%S'):<22} {'RUN_DONE':<12} {ev['runner']:<8} {delta}")
    elif ev['type'] == 'CANCEL':
        print(f"{ev['ts'].strftime('%H:%M:%S'):<22} {'CANCEL':<12} {ev['runner']:<8} {delta:<6} {ev.get('cfg','')}")
    elif ev['type'] == 'UNSCHEDULE':
        print(f"{ev['ts'].strftime('%H:%M:%S'):<22} {'UNSCHEDULE':<12} {ev['runner']:<8} {delta:<6} {ev.get('cfg','')}")
    prev_ts = ev['ts']

# Summary stats
print("\n=== SUMMARY ===")
rebalances = [e for e in events if e['type'] == 'REBALANCE']
dispatches = [e for e in events if e['type'] == 'DISPATCH']
schedules = [e for e in events if e['type'] == 'SCHEDULE']
runs = [e for e in events if e['type'] == 'RUN_DONE']
cancels = [e for e in events if e['type'] == 'CANCEL']
print(f"Rebalance events: {len(rebalances)}")
print(f"  with orch present: {sum(1 for e in rebalances if e['orch_present'])}")
print(f"  with orch absent:  {sum(1 for e in rebalances if not e['orch_present'])}")
print(f"DCA Dispatch events: {len(dispatches)}")
print(f"Runner Schedule events: {len(schedules)}")
print(f"Runner Run Done events: {len(runs)}")
print(f"Runner Cancel events: {len(cancels)}")

# Per-runner Done counts
from collections import Counter
done_per_runner = Counter(e['runner'] for e in runs)
print(f"\nDone runs per runner: {dict(done_per_runner)}")
