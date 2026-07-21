import os
import re

func_start_regex = r'^func (\w+)\(\w+ pkgconfigmodel.Setup\)'
declare_regex = r'^c\w+g\.BindEnvAndSetDefault\((.*)\)'
set_default_regex = r'^c\w+g\.SetDefault\((.*)\)'
proc_declare_regex = r'^procBindEnvAndSetDefault\(\w+, (.*)\)'
event_monitor_regex = r'^eventMonitorBindEnvAndSetDefault\(\w+, (.*)\)'

bind_env_logs = r'bindEnvAndSetLogsConfigKeys\(\w+, "([\w_\.]+)"'
bind_delegate = r'bindDelegatedAuthConfig\(\w+, "([\w_\.]+)"'


# Prefixes that begin a setting declaration. A declaration may span several lines (the arguments, a
# `[]string{...}` default, or a `GetPlatformDefault(map[...]{...})` value), so we accumulate lines until the
# parentheses balance and only then match the joined statement against the single-line regexes above.
DECL_START_PREFIXES = (
    'config.BindEnvAndSetDefault(',
    'cfg.BindEnvAndSetDefault(',
    'config.SetDefault(',
    'cfg.SetDefault',
    'procBindEnvAndSetDefault(',
)


def analyze_file(sourcefile):
    """Analyze the given source file, output 'hints' that include the
    order settings appear, and comments about those settings."""
    p = Processor()
    p.startfilename(sourcefile)
    within_func_init_config = False
    with open(sourcefile) as f:
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
APM_SETTINGS = os.path.join(SETTINGS_DIR, "apm_settings.go")
OTLP_SETTINGS = os.path.join(SETTINGS_DIR, "otlp_settings.go")
MRF_SETTINGS = os.path.join(SETTINGS_DIR, "multi_region_failover_settings.go")
# called from functions in initCoreAgentFull
PAR_SETTINGS = os.path.join(SETTINGS_DIR, "privateactionrunner_settings.go")
PROCESS_SETTINGS = os.path.join(SETTINGS_DIR, "process_settings.go")
# system probe
SYSPROBE_SETTINGS = os.path.join(SETTINGS_DIR, "system_probe_settings.go")


def extract_imperative_code_hints():
    return (
        analyze_file(COMMON_SETTINGS)
        + analyze_file(APM_SETTINGS)
        + analyze_file(OTLP_SETTINGS)
        + analyze_file(MRF_SETTINGS)
        + analyze_file(PAR_SETTINGS)
        + analyze_file(PROCESS_SETTINGS)
        + analyze_file(SYSPROBE_SETTINGS)
    )


class Processor:
    def __init__(self):
        self.currfunc = ''
        self.currfile = ''
        self.results = []
        self.settings = []
        self.internal_comment = []
        self.within_multiline = False
        self.accum_multiline = []

    def startfilename(self, filename):
        self.currfile = os.path.basename(filename)

    def startfunc(self, name):
        self._clear()
        self.currfunc = name

    def _clear(self):
        self.currfunc = ''
        self.settings = []
        self.internal_comment = []

    def clearfunc(self):
        if not self.settings:
            self._clear()
            return
        self.results.append(
            {
                'func': self.currfunc,
                'filename': self.currfile,
                'settings': self.settings,
            }
        )
        self._clear()

    @staticmethod
    def _paren_balance(text):
        """Net count of unbalanced '(' in text, ignoring parentheses inside string and raw-string literals."""
        depth = 0
        in_str = None  # '"' for interpreted strings, '`' for raw strings
        i = 0
        while i < len(text):
            c = text[i]
            if in_str == '"' and c == '\\':
                i += 2
                continue
            if in_str:
                if c == in_str:
                    in_str = None
            elif c in '"`':
                in_str = c
            elif c == '(':
                depth += 1
            elif c == ')':
                depth -= 1
            i += 1
        return depth

    def process(self, line, num):
        line = line.strip()
        if line.startswith('//'):
            self.append_internal_comment(line)
            return

        comment_pos = line.find(' // ')
        if comment_pos >= 0:
            comment_text = line[comment_pos:]
            self.append_internal_comment(comment_text)
            line = line[:comment_pos].rstrip()

        # A declaration started on a previous line: keep accumulating until the parentheses balance, then
        # dispatch the joined single-line statement.
        if self.within_multiline:
            self.accum_multiline.append(line)
            joined = ' '.join(self.accum_multiline)
            if self._paren_balance(joined) <= 0:
                self.within_multiline = False
                self.accum_multiline = []
                self.dispatch(joined)
            return

        if line.startswith('bindEnvAndSetLogsConfigKeys'):
            m = re.match(bind_env_logs, line)
            if m:
                self.register_pattern('pattern_logs_config', m.group(1))
                return

        if line.startswith('bindDelegatedAuthConfig'):
            m = re.match(bind_delegate, line)
            if m:
                self.register_pattern('pattern_delegate_auth', m.group(1))
                return

        if not line.startswith(DECL_START_PREFIXES):
            return

        # A declaration whose parentheses don't close on this line continues onto the following lines (split
        # arguments, a multi-line `[]string{...}` default, or a `GetPlatformDefault(map[...]{...})` value).
        if self._paren_balance(line) > 0:
            self.accum_multiline = [line]
            self.within_multiline = True
            return

        self.dispatch(line)

    def dispatch(self, line):
        m = re.match(declare_regex, line)
        if m:
            self.register_setting('declare', m.group(1))
            return

        m = re.match(set_default_regex, line)
        if m:
            self.register_setting('default', m.group(1))
            return

        m = re.match(proc_declare_regex, line)
        if m:
            self.register_setting('proc', m.group(1))
            return

        m = re.match(event_monitor_regex, line)
        if m:
            self.register_setting('eventmon', m.group(1))
            return

    def append_internal_comment(self, text):
        text = text.strip()
        if text.startswith('//'):
            text = text[2:]
        text = text.strip()
        # ignore this common case:
        if re.match(r'^TODO: replace by .SetDefaultAndBindEnv.', text):
            return
        self.internal_comment.append(text)

    def clean_param(self, params, index):
        if index >= len(params):
            return None
        return params[index].strip('" \'')

    def register_pattern(self, pattern_kind, setting_prefix):
        if not self.currfunc:
            raise RuntimeError('not currently in a function')
        internal_comment = '\n'.join(self.internal_comment)
        self.internal_comment = []
        self.settings.append([setting_prefix, pattern_kind, internal_comment])

    def register_setting(self, kind, params):
        if not self.currfunc:
            raise RuntimeError('not currently in a function')
        # NOTE: doesn't handle things like this:
        # `config.BindEnvAndSetDefault("histogram_aggregates", []string{"max", "median", "avg", "count"})`
        # Shouldn't matter because we don't use default values
        parts = params.split(',')

        keyname = ''
        _unused_default = None
        internal_comment = '\n'.join(self.internal_comment)
        self.internal_comment = []

        if kind in ['declare', 'default', 'proc', 'eventmon']:
            keyname = self.clean_param(parts, 0)
            _unused_default = self.clean_param(parts, 1)
        else:
            raise RuntimeError('unknown kind: %s' % kind)
        self.settings.append([keyname, kind, internal_comment])

    def finish(self):
        return
