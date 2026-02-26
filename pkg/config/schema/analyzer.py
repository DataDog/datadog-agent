import argparse
import collections
import json
import re


def read_file(filename):
    fp = open(filename, 'r')
    content = fp.read()
    fp.close()
    return content


def write_file(filename, content):
    fout = open(filename, 'w')
    fout.write(content)
    fout.close()


config_setup_func_names = [
    'InitConfig',
	'agent',
	'fips',
	'dogstatsd',
	'forwarder',
	'aggregator',
	'serializer',
	'serverless',
	'setupAPM',
	'OTLP',
	'setupMultiRegionFailover',
	'telemetry',
	'autoconfig',
	'remoteconfig',
	'logsagent',
	'containerSyspath',
	'containerd',
	'cri',
	'kubernetes',
	'cloudfoundry',
	'debugging',
	'vector',
	'podman',
	'fleet',
	'autoscaling',
]


func_start_regex = r'^func (\w+)\(config pkgconfigmodel.Setup\)'


def analyze_file(sourcefile):
    p = Processor()
    within_func_init_config = False
    content = read_file(sourcefile)
    for i, line in enumerate(content.split('\n')):
        num = i + 1
        m = re.match(func_start_regex, line)
        if m:
            within_func_init_config = True
            p.startfunc(m.group(1))
            continue
        elif line.startswith('}'):
            within_func_init_config = False
            p.clearfunc()
            continue

        if line == '':
            continue

        if within_func_init_config:
            p.process(line, num)
    p.finish()
    return p.results


class Processor():
    def __init__(self):
        self.regexDeclare = r'^config.BindEnvAndSetDefault\((.*)\)'
        self.regexEnv = r'^config.BindEnv\((.*)\)'
        self.regexKnown = r'^config.SetKnown\((.*)\)'
        self.regexDefault = r'^config.SetDefault\((.*)\)'
        self.currfunc = ''
        self.results = collections.defaultdict(list)
        self.num_fail = 0

    def startfunc(self, name):
        self.currfunc = name

    def clearfunc(self):
        self.currfunc = ''

    def process(self, line, num):
        line = line.strip()
        if line.startswith('//'):
            return

        m = re.match(self.regexDeclare, line)
        if m:
            self.registerSetting('declare', m.group(1))
            return

        m = re.match(self.regexEnv, line)
        if m:
            self.registerSetting('env', m.group(1))
            return

        m = re.match(self.regexKnown, line)
        if m:
            self.registerSetting('known', m.group(1))
            return

        m = re.match(self.regexDefault, line)
        if m:
            self.registerSetting('default', m.group(1))
            return

        print('** FAIL [%d]: %s' % (num, line))
        self.num_fail += 1

    def registerSetting(self, kind, params):
        if not self.currfunc:
            raise RuntimeError('not currently in a function')
        parts = params.split(',')
        keyname = parts[0].strip('"')
        other = parts[1:]
        self.results[self.currfunc].append([keyname, kind, other])

    def finish(self):
        num_declare = 0
        num_env     = 0
        num_known   = 0
        num_default = 0
        for func in self.results:
            print('func %s {' % func)
            table = self.results[func]
            for row in table:
                if row[1] == 'declare':
                    num_declare += 1
                elif row[1] == 'env':
                    num_env += 1
                elif row[1] == 'known':
                    num_known += 1
                elif row[1] == 'default':
                    num_default += 1
                print('  %s %s %s' % (row[0], row[1], row[2]))
            print('}')
        print('Fail:    %s' % self.num_fail)
        print('Declare: %s' % num_declare)
        print('Env:     %s' % num_env)
        print('Known:   %s' % num_known)
        print('Default: %s' % num_default)


def main():
    argparser = argparse.ArgumentParser()
    argparser.add_argument('--source', dest='source')
    argparser.add_argument('--outhints', dest='hints')
    args = argparser.parse_args()
    res = analyze_file(args.source)
    write_file(args.hints, json.dumps(res))


if __name__ == '__main__':
    main()