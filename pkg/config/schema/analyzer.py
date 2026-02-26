import argparse
import re


def read_file(filename):
    fp = open(filename, 'r')
    content = fp.read()
    fp.close()
    return content


def analyze_file(sourcefile):
    p = Processor()
    within_func_init_config = False
    content = read_file(sourcefile)
    for i, line in enumerate(content.split('\n')):
        num = i + 1
        if line.startswith('func InitConfig(config'):
            within_func_init_config = True
            continue
        elif line.startswith('}'):
            within_func_init_config = False
            continue

        if line == '':
            continue

        if within_func_init_config:
            p.process(line, num)
    p.finish()


class Processor():
    def __init__(self):
        self.regexDeclare = r'^config.BindEnvAndSetDefault\((.*)\)'
        self.regexEnv = r'^config.BindEnv\((.*)\)'
        self.regexKnown = r'^config.SetKnown\((.*)\)'
        self.regexDefault = r'^config.SetDefault\((.*)\)'
        self.result = []
        self.num_fail = 0

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
        parts = params.split(',')
        keyname = parts[0]
        other = parts[1:]
        self.result.append([keyname, kind, other])

    def finish(self):
        num_declare = 0
        num_env     = 0
        num_known   = 0
        num_default = 0
        for r in self.result:
            if r[1] == 'declare':
                num_declare += 1
            elif r[1] == 'env':
                num_env += 1
            elif r[1] == 'known':
                num_known += 1
            elif r[1] == 'default':
                num_default += 1
            print('%s %s %s' % (r[0], r[1], r[2]))
        print('Fail:    %s' % self.num_fail)
        print('Declare: %s' % num_declare)
        print('Env:     %s' % num_env)
        print('Known:   %s' % num_known)
        print('Default: %s' % num_default)


def main():
    argparser = argparse.ArgumentParser()
    argparser.add_argument('--source', dest='source')
    args = argparser.parse_args()
    analyze_file(args.source)


if __name__ == '__main__':
    main()