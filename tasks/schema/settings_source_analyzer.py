import collections
import os
import re


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


func_start_regex = r'^func (\w+)\(config pkgconfigmodel.Setup\)'
declare_regex = r'^config.BindEnvAndSetDefault\((.*)\)'
declare_multiline_regex = r'^config.BindEnvAndSetDefault\(([^)]+){$'
bindenv_regex = r'^config.BindEnv\((.*)\)'
set_known_regex = r'^config.SetKnown\((.*)\)'
set_default_regex = r'^config.SetDefault\((.*)\)'
proc_declare_regex = r'^procBindEnvAndSetDefault\(config, (.*)\)'


def analyze_file(sourcefile):
    p = Processor()
    p.startfilename(sourcefile)
    within_func_init_config = False
    with open(sourcefile, "r") as f:
        content = f.read()
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


SETTINGS_DIR = os.path.join("pkg", "config", "setup")
# initCoreAgentFull + commonConfigComponents
COMMON_SETTINGS = os.path.join(SETTINGS_DIR, "common_settings.go")
# declared in commonConfigComponents
APM_SETTINGS = os.path.join(SETTINGS_DIR, "apm.go")
OTLP_SETTINGS = os.path.join(SETTINGS_DIR, "otlp.go")
MRF_SETTINGS = os.path.join(SETTINGS_DIR, "multi_region_failover.go")
# called from functions in initCoreAgentFull
PAR_SETTINGS = os.path.join(SETTINGS_DIR, "privateactionrunner.go")
PROCESS_SETTINGS = os.path.join(SETTINGS_DIR, "process.go")
# system probe
SYSPROBE_SETTINGS = os.path.join(SETTINGS_DIR, "system_probe.go")


def extract_imperative_code_hints():
    return (
        analyze_file(COMMON_SETTINGS) +
        analyze_file(APM_SETTINGS) +
        analyze_file(OTLP_SETTINGS) +
        analyze_file(MRF_SETTINGS) +
        analyze_file(PAR_SETTINGS) +
        analyze_file(PROCESS_SETTINGS) +
        analyze_file(SYSPROBE_SETTINGS))


class Processor():
    def __init__(self):
        self.currfunc = ''
        self.currfile = ''
        self.results = []
        self.settings = []
        self.num_fail = 0
        self.internal_comment = []
        self.within_multiline = False
        self.accum_multiline = []
        self.begin_multiline = None
        self.curr_linenum = 0

    def startfilename(self, filename):
        self.currfile = os.path.basename(filename)

    def startfunc(self, name):
        self.currfunc = name
        self.settings = []

    def clearfunc(self):
        if not self.settings:
            self.currfunc = ''
            self.settings = []
            return
        self.results.append({
            'func': self.currfunc,
            'filename': self.currfile,
            'settings': self.settings,
        })
        self.currfunc = ''
        self.settings = []

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

        m = re.match(declare_regex, line)
        if m:
            self.register_setting('declare', m.group(1))
            return

        m = re.match(declare_multiline_regex, line)
        if m:
            self.accum_multiline = []
            self.within_multiline = True
            self.register_setting('declare_multiline', m.group(1))
            return

        m = re.match(bindenv_regex, line)
        if m:
            self.register_setting('env', m.group(1))
            return

        m = re.match(set_known_regex, line)
        if m:
            self.register_setting('known', m.group(1))
            return

        m = re.match(set_default_regex, line)
        if m:
            self.register_setting('default', m.group(1))
            return

        m = re.match(proc_declare_regex, line)
        if m:
            self.register_setting('proc', m.group(1))
            return

        #print('** FAIL [%d]: %s' % (num, line))
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
        elif kind == 'proc':
            keyname = self.clean_param(parts, 0)
            unused_default = self.clean_param(parts, 1)
        else:
            raise RuntimeError('unknown kind: %s' % kind)
        # unused: envvars
        self.settings.append([keyname, kind, internal_comment])

    def complete_multiline(self):
        keyname = self.begin_multiline['keyname']
        kind = self.begin_multiline['kind']
        envvars = self.begin_multiline['envvars']
        # unused: envvars
        self.settings.append([keyname, kind, ''])

    def finish(self):
        return
