import re

p = re.compile(r'#define\s+\w+\s*\(\w+\s*\+\s*(\d+)\)\s*\/\*\s*([^*.]+.)')
print('const std::map <int, std::wstring> lmerrors = {')
with open('C:\\Program Files (x86)\\Windows Kits\\8.1\\Include\\shared\\lmerr.h', 'r') as file:
    match = p.finditer(file.read())
    if match:
        for m in match:
            print(f'\t{{{m.group(1)}, L"{m.group(2)}"}},')
print('};')
