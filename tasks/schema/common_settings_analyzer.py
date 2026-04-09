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
    'initCoreAgentFull',
	'agent',
	'fleet',
	'autoscaling',
	'fips',
	'remoteconfig',
	'autoconfig',
	'containerSyspath',
	'debugging',
	'telemetry',
	'serializer',
	'aggregator',
	'serverless',
	'forwarder',
	'dogstatsd',
	'logsagent',
	'vector',
	'cloudfoundry',
	'containerd',
	'cri',
	'kubernetes',
	'podman',
]


# apm.go
#	'setupAPM',
# otlp.go
#	'OTLP',
# multi_region_failover.go
#	'setupMultiRegionFailover',


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
        self.regexDeclareMultiline = r'^config.BindEnvAndSetDefault\(([^)]+){$'
        self.regexEnv = r'^config.BindEnv\((.*)\)'
        self.regexKnown = r'^config.SetKnown\((.*)\)'
        self.regexDefault = r'^config.SetDefault\((.*)\)'
        self.currfunc = ''
        self.results = collections.defaultdict(list)
        self.num_fail = 0
        self.internal_comment = []
        self.within_multiline = False
        self.accum_multiline = []
        self.begin_multiline = None
        self.curr_linenum = 0

    def startfunc(self, name):
        self.currfunc = name

    def clearfunc(self):
        self.currfunc = ''

    def process(self, line, num):
        self.curr_linenum = num
        line = line.strip()
        if line.startswith('//'):
            self.append_internal_comment(line)
            return
        
        comment_pos = line.find(' // ')
        if comment_pos >= 0:
            line = line[:comment_pos]

        if self.within_multiline:
            if line.startswith('}'):
                self.complete_multiline()
                self.within_multiline = False
                return
            self.accum_multiline.append(line)
            return  

        # TODO: Handle these functions
        if line.startswith('bindEnvAndSetLogsConfigKeys'):
            return
        if line.startswith('bindDelegatedAuthConfig'):
            return
        if line.startswith('pkgconfigmodel.AddOverrideFunc'):
            return
        if line.startswith('config.ParseEnvAs'):
            return       
        if line.startswith('setupProcesses'):
            return
        if line.startswith('setupPrivateActionRunner'):
            return       

        m = re.match(self.regexDeclare, line)
        if m:
            self.register_setting('declare', m.group(1))
            return

        m = re.match(self.regexDeclareMultiline, line)
        if m:
            self.accum_multiline = []
            self.within_multiline = True
            self.register_setting('declare_multiline', m.group(1))
            return

        m = re.match(self.regexEnv, line)
        if m:
            self.register_setting('env', m.group(1))
            return

        m = re.match(self.regexKnown, line)
        if m:
            self.register_setting('known', m.group(1))
            return

        m = re.match(self.regexDefault, line)
        if m:
            self.register_setting('default', m.group(1))
            return

        print('** FAIL [%d]: %s' % (num, line))
        self.num_fail += 1

    def append_internal_comment(self, text):
        if text.startswith('//'):
            text = text[2:]
        text = text.strip()
        self.internal_comment.append(text)

    def clean_param(self, params, index):
        if index >= len(params):
            return None
        return params[index].strip('" \'')

    def clean_env_vars(self, params, index):
        if index >= len(params):
            return None
        elems = params[index:]
        elems = [elems.strip('" \'') for elems in elems]
        for ev in elems:
            if not ev.startswith('DD_'):
                print('*** [ERROR] unknown text instead of env vars: %s' % params[index:])
                return []
        return elems

    def register_setting(self, kind, params):
        if not self.currfunc:
            raise RuntimeError('not currently in a function')
        # TODO: doesn't handle things like this:
        # `config.BindEnvAndSetDefault("histogram_aggregates", []string{"max", "median", "avg", "count"})`
        parts = params.split(',')

        keyname = ''
        unused_default = None
        envvars = []
        internal_comment = '\n'.join(self.internal_comment)
        self.internal_comment = []

        if kind == 'declare':
            keyname = self.clean_param(parts, 0)
            unused_default = self.clean_param(parts, 1)
            envvars = self.clean_env_vars(parts, 2)
        elif kind == 'env':
            keyname = self.clean_param(parts, 0)
            envvars = self.clean_env_vars(parts, 1)
        elif kind == 'known':
            keyname = self.clean_param(parts, 0)
        elif kind == 'default':
            keyname = self.clean_param(parts, 0)
            unused_default = self.clean_param(parts, 1)
        elif kind == 'declare_multiline':            
            keyname = self.clean_param(parts, 0)
            self.begin_multiline = {'keyname': keyname, 'kind': kind, 'envvars': envvars}
            return
        else:
            raise RuntimeError('unknown kind: %s' % kind)

        # unused: envvars
        self.results[self.currfunc].append([keyname, kind, internal_comment])

    def complete_multiline(self):
        keyname = self.begin_multiline['keyname']
        kind = self.begin_multiline['kind']
        envvars = self.begin_multiline['envvars']
        # unused: envvars
        self.results[self.currfunc].append([keyname, kind, ''])

    def finish(self):
        num_declare = 0
        num_env     = 0
        num_known   = 0
        num_default = 0
        for func in self.results:
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
        print('Fail:    %s' % self.num_fail)
        print('Declare: %s' % num_declare)
        print('Env:     %s' % num_env)
        print('Known:   %s' % num_known)
        print('Default: %s' % num_default)
