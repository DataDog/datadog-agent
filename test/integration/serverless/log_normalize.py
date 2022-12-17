import argparse
import json
import re


def normalize_metrics(stage):
    return [
        replace(r'raise Exception', r'\n'),
        require(r'BEGINMETRIC.*ENDMETRIC'),
        exclude(r'BEGINMETRIC'),
        exclude(r'ENDMETRIC'),
        replace(r'(ts":)[0-9]{10}', r'\1XXX'),
        replace(r'(min":)[0-9\.e\-]{1,30}', r'\1XXX'),
        replace(r'(max":)[0-9\.e\-]{1,30}', r'\1XXX'),
        replace(r'(cnt":)[0-9\.e\-]{1,30}', r'\1XXX'),
        replace(r'(avg":)[0-9\.e\-]{1,30}', r'\1XXX'),
        replace(r'(sum":)[0-9\.e\-]{1,30}', r'\1XXX'),
        replace(r'(k":\[)[0-9\.e\-]{1,30}', r'\1XXX'),
        replace(r'(datadog-nodev)[0-9]+\.[0-9]+\.[0-9]+', r'\1X.X.X'),
        replace(r'(datadog_lambda:v)[0-9]+\.[0-9]+\.[0-9]+', r'\1X.X.X'),
        replace(r'dd_lambda_layer:datadog-go[0-9.]{1,}', r'dd_lambda_layer:datadog-gox.x.x'),
        replace(r'(dd_lambda_layer:datadog-python)[0-9_]+\.[0-9]+\.[0-9]+', r'\1X.X.X'),
        replace(r'(serverless.lambda-extension.integration-test.count)[0-9\.]+', r'\1'),
        replace(r'(architecture:)(x86_64|arm64)', r'\1XXX'),
        replace(stage, 'XXXXXX'),
        exclude(r'[ ]$'),
    ]


def normalize_logs(stage):
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
        replace(stage, 'XXXXXX'),
        replace(r'(architecture:)(x86_64|arm64)', r'\1XXX'),
        sort_by(lambda log: log["message"]["message"]),
        # ignore a Lambda error that occurs sporadically for log-csharp see here for more info:
        # https://repost.aws/questions/QUq2OfIFUNTCyCKsChfJLr5w/lambda-function-working-locally-but-crashing-on-aws
        # TODO
        # perl -n -e "print unless /LAMBDA_RUNTIME Failed to get next invocation. No Response from endpoint/ or \
        #  /An error occurred while attempting to execute your code.: LambdaException/ or \
        #  /terminate called after throwing an instance of 'std::logic_error'/ or \
        #  /basic_string::_M_construct null not valid/" |
    ]


def normalize_traces(stage):
    return [
        require(r'BEGINTRACE.*ENDTRACE'),
        exclude(r'BEGINTRACE'),
        exclude(r'ENDTRACE'),
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
        replace(stage, 'XXXXXX'),
        exclude(r'[ ]$'),
    ]


#####################
# BEGIN NORMALIZERS #
#####################


def replace(pattern, repl):
    comp = re.compile(pattern, flags=re.DOTALL)

    def _replace(log):
        return comp.sub(repl, log)

    return _replace


def exclude(pattern):
    return replace(pattern, '')


def require(pattern):
    comp = re.compile(pattern, flags=re.DOTALL)

    def _require(log):
        match = comp.search(log)
        if not match:
            return ''
        return match.group(0)

    return _require


def sort_by(key):
    def _sort(log):
        log_json = json.loads(log, strict=False)
        log_sorted = sorted(log_json, key=key)
        return json.dumps(log_sorted)

    return _sort


###################
# END NORMALIZERS #
###################


def normalize(log, typ, stage):
    for normalizer in get_normalizers(typ, stage):
        log = normalizer(log)
    return format_json(log)


def get_normalizers(typ, stage):
    if typ == 'metrics':
        return normalize_metrics(stage)
    elif typ == 'logs':
        return normalize_logs(stage)
    elif typ == 'traces':
        return normalize_traces(stage)
    else:
        raise ValueError(f'invalid type "{typ}"')


def format_json(log):
    if not log:
        log = '{"error":"normalization returned empty payload"}'
    try:
        return json.dumps(json.loads(log, strict=False), indent=2)
    except Exception as e:
        err = {
            "error": f"normalization raised exception: [{e.__class__.__name__}] {e}",
            "normalized-logs": log,
        }
        return json.dumps(err, indent=2)


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument('--type', required=True)
    parser.add_argument('--logs', required=True)
    parser.add_argument('--stage', required=True)
    return parser.parse_args()


if __name__ == '__main__':
    try:
        args = parse_args()
        print(normalize(args.logs, args.type, args.stage))
    except Exception as e:
        err = {
            "error": f"normalization raised exception: [{e.__class__.__name__}] {e}",
            "original-logs": args.logs,
        }
        err_json = json.dumps(err, indent=2)
        print(err_json)
        exit(1)
