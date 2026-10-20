#!/usr/bin/env python3
"""Reassemble complete heap snapshots from one or more SMP target JSONL logs."""

import argparse
import base64
import gzip
import hashlib
import json
from collections import defaultdict
from pathlib import Path


def varint(data, pos):
    value = shift = 0
    while True:
        byte = data[pos]
        pos += 1
        value |= (byte & 127) << shift
        if byte < 128:
            return value, pos
        shift += 7
        if shift >= 70:
            raise ValueError('invalid protobuf varint')


def fields(data):
    pos = 0
    while pos < len(data):
        tag, pos = varint(data, pos)
        kind = tag & 7
        if kind == 0:
            value, pos = varint(data, pos)
        elif kind == 2:
            length, pos = varint(data, pos)
            value, pos = data[pos : pos + length], pos + length
        elif kind in (1, 5):
            length = 8 if kind == 1 else 4
            value, pos = data[pos : pos + length], pos + length
        else:
            raise ValueError(f'unsupported wire type {kind}')
        yield tag >> 3, value


def numbers(value):
    if isinstance(value, int):
        return [value]
    out, pos = [], 0
    while pos < len(value):
        number, pos = varint(value, pos)
        out.append(number)
    return out


def summarize(profile):
    outer = list(fields(gzip.decompress(profile)))
    strings = [v.decode() for n, v in outer if n == 6]
    types = [strings[dict(fields(v))[1]] for n, v in outer if n == 1]
    functions = {}
    for n, value in outer:
        if n == 5:
            record = dict(fields(value))
            functions[record[1]] = strings[record[2]]
    locations = {}
    for n, value in outer:
        if n == 4:
            record = list(fields(value))
            locations[dict(record)[1]] = [dict(fields(v))[1] for k, v in record if k == 4]
    index = types.index('inuse_space')
    flat, cumulative = defaultdict(int), defaultdict(int)
    total = 0
    for n, value in outer:
        if n != 2:
            continue
        record = list(fields(value))
        stack = [x for k, v in record if k == 1 for x in numbers(v)]
        values = [x for k, v in record if k == 2 for x in numbers(v)]
        size = values[index]
        total += size
        names = [functions[f] for loc in stack for f in locations.get(loc, [])]
        if names:
            flat[names[0]] += size
        for name in set(names):
            cumulative[name] += size

    def rank(mapping):
        return [
            {'function': name, 'bytes': value}
            for name, value in sorted(mapping.items(), key=lambda pair: -pair[1])[:30]
        ]

    return {'sample_types': types, 'inuse_space_bytes': total, 'flat': rank(flat), 'cumulative': rank(cumulative)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('logs', nargs='+', type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    groups = defaultdict(list)
    memory = []
    for path in args.logs:
        for line in path.read_text().splitlines():
            try:
                obj = json.loads(line)
                if 'message' in obj:
                    obj = json.loads(obj['message'])
            except (ValueError, TypeError):
                continue
            if obj.get('diagnostic') != 'endpoint_routing_heap':
                continue
            if 'sequence' in obj:
                groups[
                    (obj.get('session', hashlib.sha256(str(path.resolve()).encode()).hexdigest()[:12]), obj['sequence'])
                ].append(obj)
            else:
                memory.append(obj)
    manifest = []
    for (session, seq), rows in groups.items():
        starts = [r for r in rows if r['kind'] == 'heap_start']
        ends = [r for r in rows if r['kind'] == 'heap_end']
        if not starts or not ends:
            raise ValueError(f'incomplete heap snapshot {session}/{seq}')
        start = starts[0]
        chunks = {r['index']: r['data'] for r in rows if r['kind'] == 'heap_chunk'}
        if set(chunks) != set(range(start['chunks'])):
            raise ValueError(f'missing chunks {session}/{seq}')
        data = base64.b64decode(''.join(chunks[i] for i in range(start['chunks'])))
        assert hashlib.sha256(data).hexdigest() == start['sha256'] == ends[0]['sha256']
        name = f'{session}-{seq}'
        (args.output / (name + '.pprof')).write_bytes(data)
        summary = summarize(data)
        (args.output / (name + '.json')).write_text(json.dumps(summary, indent=2))
        manifest.append(dict(start, file=name + '.pprof', inuse_space_bytes=summary['inuse_space_bytes']))
    if not manifest:
        raise ValueError('no complete heap snapshots found')
    (args.output / 'manifest.json').write_text(json.dumps(manifest, indent=2))
    (args.output / 'runtime.json').write_text(json.dumps(memory, indent=2))
    print(json.dumps({'snapshots': len(manifest), 'runtime_samples': len(memory), 'output': str(args.output)}))


if __name__ == '__main__':
    main()
