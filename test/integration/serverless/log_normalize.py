import argparse
import re
import sys

metric_replaces = [
        (r'raise Exception', r'\n'),
        (r'(BEGINMETRIC)', r'\1'),
        (r'(ENDMETRIC)', r'\1'),
        (r'(ts":)[0-9]{10}', r'\1XXX'),
        (r'(min":)[0-9\.e\-]{1,30}', r'\1XXX'),
        (r'(max":)[0-9\.e\-]{1,30}', r'\1XXX'),
        (r'(cnt":)[0-9\.e\-]{1,30}', r'\1XXX'),
        (r'(avg":)[0-9\.e\-]{1,30}', r'\1XXX'),
        (r'(sum":)[0-9\.e\-]{1,30}', r'\1XXX'),
        (r'(k":\[)[0-9\.e\-]{1,30}', r'\1XXX'),
        (r'(datadog-nodev)[0-9]+\.[0-9]+\.[0-9]+', r'\1X\.X\.X'),
        (r'(datadog_lambda:v)[0-9]+\.[0-9]+\.[0-9]+', r'\1X\.X\.X'),
        (r'dd_lambda_layer:datadog-go[0-9.]{1,}', r'dd_lambda_layer:datadog-gox.x.x'),
        (r'(dd_lambda_layer:datadog-python)[0-9_]+\.[0-9]+\.[0-9]+', r'\1X\.X\.X'),
        (r'(serverless.lambda-extension.integration-test.count)[0-9\.]+', r'\1'),
        (r'(architecture:)(x86_64|arm64)', r'\1XXX'),
        (r'$stage', r'XXXXXX'),
        (r'[ ]$', r''),
]
metric_excludes = [
        r'BEGINLOG.*',
        r'BEGINTRACE.*',
]
metric_requires = [
        r'BEGINMETRIC.*',
]

log_replaces = [
        (r'BEGINLOG', '\1'),
        (r'ENDLOG', '\1'),
        (r'("timestamp": )\d{13}', '\1"XXX"'),
        (r'("timestamp": )\d{13}', '\1"XXX"'),
        (r'\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}:\d{3}', 'TIMESTAMP'),
        (r'\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z', 'TIMESTAMP'),
        (r'\d{4}\/\d{2}\/\d{2}\s\d{2}:\d{2}:\d{2}', 'TIMESTAMP'),
        (r'\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}', 'TIMESTAMP'),
        (r'"timestamp":\d{13},', '\1'),
        (r'([a-zA-Z0-9]{8}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{12})', '\0XXX'),
        (r'([a-zA-Z0-9]{8}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{4}-[a-zA-Z0-9]{12})', '\0XXX'),
        (r'$stage', 'STAGE'),
        (r'(architecture:)(x86_64|arm64)', '\1XXX'),
        # ignore a Lambda error that occurs sporadically for log-csharp see here for more info:
        # https://repost.aws/questions/QUq2OfIFUNTCyCKsChfJLr5w/lambda-function-working-locally-but-crashing-on-aws
        # TODO
        # perl -n -e "print unless /LAMBDA_RUNTIME Failed to get next invocation. No Response from endpoint/ or \
        #  /An error occurred while attempting to execute your code.: LambdaException/ or \
        #  /terminate called after throwing an instance of 'std::logic_error'/ or \
        #  /basic_string::_M_construct null not valid/" |
]
log_excludes = [
        r'BEGINMETRIC.*',
        r'BEGINTRACE.*',
]
log_requires = [
        r'BEGINLOG',
]

trace_replaces = [
        (r'BEGINTRACE', '\1'),
        (r'ENDTRACE', '\1'),
        (r'(ts":)[0-9]{10}', '\1XXX'),
        (r'((startTime|endTime|traceID|trace_id|span_id|parent_id|start|system.pid)":)[0-9]+', '\1null'),
        (r'((tracer_version|language_version)":)["a-zA-Z0-9~\-\.\_]+', '\1null'),
        (r'(duration":)[0-9]+', '\1null'),
        (r'((datadog_lambda|dd_trace)":")[0-9]+\.[0-9]+\.[0-9]+', '\1X\.X\.X'),
        (r'(,"request_id":")[a-zA-Z0-9\-,]+"', '\1null"'),
        (r'(,"runtime-id":")[a-zA-Z0-9\-,]+"', '\1null"'),
        (r'(,"system.pid":")[a-zA-Z0-9\-,]+"', '\1null"'),
        (r'("_dd.no_p_sr":)[0-9\.]+', '\1null'),
        (r'("architecture":)"(x86_64|arm64)"', '\1"XXX"'),
        (r'("process_id":)[0-9]+', '\1null'),
        (r'$stage', 'XXXXXX'),
        (r'[ ]$', ''),
]
trace_excludes = [
        r'BEGINMETRIC.*',
        r'BEGINLOG.*',
]
trace_requires = [
        r'BEGINTRACE',
]

def replace(logs, typ):
    requires, excludes, replaces = get_updaters(typ)
    for log in logs:
        for pattern in requires:
            if not pattern.search(log):
                log = ''
        for pattern in excludes:
            if pattern.search(log):
                log = ''
        for pattern, replace in replaces:
            log = pattern.sub(replace, log)
        if log:
            yield log

def get_updaters(typ):
    if typ == 'metric':
        requires, excludes, replaces = (
                metric_requires, metric_excludes, metric_replaces)
    elif typ == 'log':
        requires, excludes, replaces = (
                log_requires, log_excludes, log_replaces)
    elif typ == 'trace':
        requires, excludes, replaces = (
                trace_requires, trace_excludes, trace_replaces)
    else:
        raise ValueError(f'invalid type "{typ}"')

    replaces = [(re.compile(i), j) for i, j in replaces]
    excludes = [re.compile(i) for i in excludes]
    requires = [re.compile(i) for i in requires]

    return requires, excludes, replaces

def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument('--type', required=True)
    parser.add_argument('--logs', required=True)
    return parser.parse_args()

if __name__ == '__main__':
    args = parse_args()
    for line in replace(args.logs.split('\n'), args.type):
        print(line)
