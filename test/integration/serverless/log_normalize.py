from __future__ import annotations

import argparse
import json
import os
import re
import traceback


def normalize_metrics(stage, aws_account_id):
    def clear_dogsketches(log):
        log["dogsketches"] = []

    def sort_tags(log):
        log["tags"].sort()

    def metric_sort_key(log):
        return (log["metric"], "cold_start:true" in log["tags"])

    return [
        replace(r'raise Exception', r'\n'),
        require(r'BEGINMETRIC.*ENDMETRIC'),
        exclude(r'BEGINMETRIC'),
        exclude(r'ENDMETRIC'),
        replace(r'(datadog-nodev)[0-9]+\.[0-9]+\.[0-9]+', r'\1X.X.X'),
        replace(r'(datadog_lambda:v)[0-9]+\.[0-9]+\.[0-9]+', r'\1X.X.X'),
        replace(r'dd_lambda_layer:datadog-go[0-9.]{1,}', r'dd_lambda_layer:datadog-gox.x.x'),
        replace(r'(dd_lambda_layer:datadog-python)[0-9_]+\.[0-9]+\.[0-9]+', r'\1X.X.X'),
        replace(r'(serverless.lambda-extension.integration-test.count)[0-9\.]+', r'\1'),
        replace(r'(architecture:)(x86_64|arm64)', r'\1XXX'),
        replace(stage, 'XXXXXX'),
        replace(aws_account_id, '############'),
        exclude(r'[ ]$'),
        foreach(clear_dogsketches),
        foreach(sort_tags),
        sort_by(metric_sort_key),
    ]


def normalize_logs(stage, aws_account_id):
    rmvs = (
        # TODO: these messages may be an indication of a real problem and
        # should be investigated
        "TIMESTAMP http: proxy error: context canceled",
    )

    def rm_extra_items_key(log):
        return any(rmv in log["message"]["message"] for rmv in rmvs)

    def sort_tags(log):
        tags = log["ddtags"].split(',')
        tags.sort()
        log["ddtags"] = ','.join(tags)

    def log_sort_key(log):
        return log["message"]["message"]

    return [
        require(r'BEGINLOG.*ENDLOG'),
        exclude(r'BEGINLOG'),
        exclude(r'ENDLOG'),
        replace(r'("timestamp":\s*?)\d{13}', r'\1"XXX"'),
        replace(r'\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}:\d{3}', 'TIMESTAMP'),
        replace(r'\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z', 'TIMESTAMP'),
        replace(r'\d{4}\/\d{2}\/\d{2}\s\d{2}:\d{2}:\d{2}', 'TIMESTAMP'),
        replace(r'\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}', 'TIMESTAMP'),
        replace(r'([a-zA-Z0-9]{8}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{12})', r'XXX'),
        replace(r'"REPORT RequestId:.*?"', '"REPORT"'),
        replace(stage, 'XXXXXX'),
        replace(aws_account_id, '############'),
        replace(r'(architecture:)(x86_64|arm64)', r'\1XXX'),
        rm_item(rm_extra_items_key),
        foreach(sort_tags),
        sort_by(log_sort_key),
    ]


def normalize_traces(stage, aws_account_id):
    def trace_sort_key(log):
        name = log['chunks'][0]['spans'][0]['name']
        cold_start = log['chunks'][0]['spans'][0]['meta'].get('cold_start')
        return name, cold_start

    def sort__dd_tags_container(log):
        tags = log.get("tags") or {}
        tags = tags.get("_dd.tags.container")
        if not tags:
            return
        tags = tags.split(',')
        tags.sort()
        log["tags"]["_dd.tags.container"] = ','.join(tags)
        return log

    return [
        require(r'BEGINTRACE.*ENDTRACE'),
        exclude(r'BEGINTRACE'),
        exclude(r'ENDTRACE'),
        exclude(r'"_dd.install.id":"[a-zA-Z0-9\-]+",'),
        exclude(r'"_dd.install.time":"[0-9]+",'),
        exclude(r'"_dd.install.type":"[a-zA-Z0-9_\-]+",'),
        exclude(r'"_dd.p.tid":"[0-9a-fA-F]+",'),
        exclude(r'"_dd.tracer_hostname":"\d{1,3}(?:.\d{1,3}){3}"+,'),
        replace(r'(ts":)[0-9]{10}', r'\1XXX'),
        replace(r'((startTime|endTime|traceID|trace_id|span_id|parent_id|start|system.pid)":)[0-9]+', r'\1null'),
        replace(r'((tracer_version|language_version)":)["a-zA-Z0-9~\-\.\_]+', r'\1null'),
        replace(r'(duration":)[0-9]+', r'\1null'),
        replace(r'((datadog_lambda|dd_trace)":")[0-9]+\.[0-9]+\.[0-9]+', r'\1X.X.X'),
        replace(r'(,"request_id":")[a-zA-Z0-9\-,]+"', r'\1null"'),
        replace(r'(,"runtime-id":")[a-zA-Z0-9\-,]+"', r'\1null"'),
        replace(r'(,"system.pid":")[a-zA-Z0-9\-,]+"', r'\1null"'),
        replace(r'("_dd.no_p_sr":)[0-9\.]+', r'\1null'),
        replace(r'("architecture":)"(x86_64|arm64)"', r'\1"XXX"'),
        replace(r'("process_id":)[0-9]+', r'\1null'),
        replace(r'("otel.trace_id":")[a-zA-Z0-9]+"', r'\1null"'),
        replace(r'("_dd.p.dm":")\-[0-9]+"', r'\1null"'),
        replace(r'("_dd.otlp_sr":")[0-9\.]+"', r'\1null"'),
        replace(r'("faas.execution":")[a-zA-Z0-9-]+"', r'\1null"'),
        replace(r'("faas.instance":")[a-zA-Z0-9-/]+\[\$LATEST\][a-zA-Z0-9]+"', r'\1null"'),
        replace(stage, 'XXXXXX'),
        replace(aws_account_id, '############'),
        exclude(r'[ ]$'),
        foreach(sort__dd_tags_container),
        sort_by(trace_sort_key),
    ]


def normalize_proxy():
    return [find(r'Using proxy .*? for URL .*?\'.*?\'')]


def normalize_appsec(stage):
    def select__dd_appsec_json(log):
        """Selects the content of spans.*.meta.[_dd.appsec.json] which is
        unfortunately an embedded JSON string value, so it's parsed out.
        """

        entries = []

        for chunk in log["chunks"]:
            for span in chunk.get("spans") or []:
                meta = span.get("meta") or {}
                data = meta.get("_dd.appsec.json")
                if data is None:
                    continue
                parsed = json.loads(data, strict=False)
                # The triggers may appear in any order, so we sort them by rule ID
                parsed["triggers"] = sorted(parsed["triggers"], key=lambda x: x["rule"]["id"])
                entries.append(parsed)

        return entries

    return [
        require(r'BEGINTRACE.*ENDTRACE'),
        exclude(r'BEGINTRACE'),
        exclude(r'ENDTRACE'),
        flatmap(select__dd_appsec_json),
        replace(stage, 'XXXXXX'),
    ]


#####################
# BEGIN NORMALIZERS #
#####################


def replace(pattern, repl):
    """
    Replace all substrings matching regex pattern with given replacement string
    """
    comp = re.compile(pattern, flags=re.DOTALL)

    def _replace(log):
        return comp.sub(repl, log)

    return _replace


def exclude(pattern):
    """
    Remove all substrings matching regex pattern
    """
    return replace(pattern, '')


def require(pattern):
    """
    Remove all substrings that don't match the given regex pattern
    """
    comp = re.compile(pattern, flags=re.DOTALL)

    def _require(log):
        match = comp.search(log)
        if not match:
            return ''
        return match.group(0)

    return _require


def find(pattern):
    comp = re.compile(pattern, flags=re.DOTALL)

    def _find(log):
        matches = comp.findall(log)
        if not matches:
            return ''
        return '\n'.join(set(matches))

    return _find


def foreach(fn):
    """
    Execute fn with each element of the list in order
    """

    def _foreach(log):
        logs = json.loads(log, strict=False)
        for log_item in logs:
            fn(log_item)
        return json.dumps(logs, sort_keys=True)

    return _foreach


def flatmap(fn):
    """
    Execute fn with each element of the list in order, flatten the results.
    """

    def _flat_map(log):
        logs = json.loads(log, strict=False)

        mapped = []
        for log_item in logs:
            mapped.extend(fn(log_item))

        return json.dumps(mapped, sort_keys=True)

    return _flat_map


def sort_by(key):
    """
    Sort the json entries using the given key function, requires the log string
    to be proper json and to be a list
    """

    def _sort(log):
        log_json = json.loads(log, strict=False)
        log_sorted = sorted(log_json, key=key)
        return json.dumps(log_sorted)

    return _sort


def rm_item(key):
    """
    Delete json entries from the log string using the given key function, key
    takes an item from the json list and must return boolean which is True when
    the item is to be removed and False if it is to be kept
    """

    def _rm_item(log):
        log_json = json.loads(log, strict=False)
        new_logs = [i for i in log_json if not key(i)]
        return json.dumps(new_logs)

    return _rm_item


###################
# END NORMALIZERS #
###################


def normalize(log, typ, stage, aws_account_id):
    for normalizer in get_normalizers(typ, stage, aws_account_id):
        log = normalizer(log)
    return format_json(log)


def get_normalizers(typ, stage, aws_account_id):
    if typ == 'metrics':
        return normalize_metrics(stage, aws_account_id)
    elif typ == 'logs':
        return normalize_logs(stage, aws_account_id)
    elif typ == 'traces':
        return normalize_traces(stage, aws_account_id)
    elif typ == 'appsec':
        return normalize_appsec(stage)
    elif typ == 'proxy':
        return normalize_proxy()
    else:
        raise ValueError(f'invalid type "{typ}"')


def format_json(log):
    try:
        return json.dumps(json.loads(log, strict=False), indent=2)
    except json.JSONDecodeError:
        return log


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument('--accountid', required=True)
    parser.add_argument('--type', required=True)
    parser.add_argument('--logs', required=True)
    parser.add_argument('--stage', required=True)
    return parser.parse_args()


if __name__ == '__main__':
    try:
        args = parse_args()

        if args.logs.startswith('file:'):
            with open(args.logs[5:]) as f:
                args.logs = f.read()

        print(normalize(args.logs, args.type, args.stage, args.accountid))
    except Exception as e:
        err: dict[str, str | list[str]] = {
            "error": "normalization raised exception",
        }
        # Unless explicitly specified, perform as it did historically
        if os.environ.get("TRACEBACK") == "true":
            err["message"] = str(e)
            err["backtrace"] = traceback.format_exception(type(e), e, e.__traceback__)

        err_json = json.dumps(err, indent=2)
        print(err_json)
        exit(1)
