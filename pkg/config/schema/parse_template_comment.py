#!/usr/bin/env python
import collections
import sys
import yaml
from pprint import pprint
import re


def get_indent(line):
    count = 0
    for i in line:
        if i == " ":
            count += 1
        else:
            if count % 2 != 0:
                raise Exception("Odd indent: {block}")
            return count // 2

def remove_first_pound(block):
    cleaned = []
    # edge case for the top level where the comments don't have a space after the "#" but the setting does
    if block[0].startswith("##"):
        for l in block:
            if l.startswith("# "):
                l = l[2:]
            else:
                l = l[1:]
            cleaned.append(l)
        return cleaned

    # remove first '# "
    return [l[2:] for l in block]

def get_doc(block):
    doc = []
    for idx, l in enumerate(block):
        if l.startswith("#"):
            l = l[2:]
            if l.startswith("@param") or l.startswith("@env"):
                continue
            doc.append(l)
        else:
            break
    return "\n".join(doc).strip(), block[idx:]

def get_name_and_example(block):
    # remove empty lines
    while len(block):
        if block[0] == "":
            block.pop(0)
        else:
            break

    name, rest = block[0].split(":", 1)

    example = [rest.strip()]
    example.extend(block[1:])
    return name, "\n".join(example)

def parse_block(block):
    doc, block = get_doc(block)
    name, example = get_name_and_example(block)
    return name, doc, example

def handle_header(block):
    """
    Handle header/title such as:
        #########################
        ## Basic Configuration ##
        #########################
    """
    if len(block) != 3:
        raise Exception(f"Invalid header: {block}")

    return block[1][3:-3]

def get_from_schema(currpath, field, schemaroot):
    node = schemaroot
    for s in currpath:
        if s not in node:
            return None
        node = node[s]["properties"]
    return node.get(field)

class Parser(object):

    def __init__(self):
        self.current_title = ""
        self.order = 0
        self.trackorder = collections.defaultdict(list)
        self.parents = []
        self.previous_name = ""
        self.template_section = ""
        self.template_nested_level = 0

    def current_indent(self):
        return len(self.parents)

    def handle_item(self, block, schema):
        block = remove_first_pound(block)
        indent = get_indent(block[0])

        # we remove the YAML indent
        block = [l[indent*2:] for l in block]

        name, doc, example = parse_block(block)

        # we go one level deeper
        if self.current_indent()+1 == indent:
            self.parents.append(self.previous_name)
        elif self.current_indent() == indent:
            pass
        elif self.current_indent() > indent:
            while self.current_indent() > indent:
                self.parents.pop()
        else:
            raise Exception(f"unknown indent at {self.parents}")

        self.previous_name = name

        node = get_from_schema(self.parents, name, schema)
        if node is None:
            # element form the config not in the schema. The templates contains information for both the core agent and
            # the system-probe
            return

        node["description"] = doc
        node["visibility"] = "public"
        if self.current_title != "":
            node["title"] = self.current_title
            self.current_title = ""
        if node.get("node_type", "") != "section":
            if example and node.get("default") != example:
                node["example"] = example

        tags = node.get("tags", [])
        if self.template_section != "":
            tags.append(f"template_section:{self.template_section}")
        tags.append(f"template_section_order:{self.order}")
        node["tags"] = tags

        self.order += 1
        self.trackorder['.'.join(self.parents)].append(name)

    def handle_template_section(self, line):
        if line.startswith("{{ end "):
            self.template_nested_level -= 1
            if self.template_nested_level == 0:
                self.template_section = ""
            return

        res = re.search("{{-?\\s*if \\.([a-zA-Z]+)\\s*-?}}", line)
        new_block = False
        if res:
            self.template_section = res.group(1)
            # we found a new template block. This means that the previous setting is done
            new_block == True
        self.template_nested_level += 1
        return new_block

    def run(self, template, schema):
        block = []
        for line in template.split("\n"):
            if line.startswith("{{"):
                new_block = self.handle_template_section(line)
                continue

            if line != "":
                block.append(line)
            else:
                if len(block) == 0:
                    continue

                #pprint(block)
                if block[0].startswith("###"):
                    self.current_title = handle_header(block)
                else:
                    self.handle_item(block, schema)
                block = []


# Each node should use the same key order
def nice_key_order(obj):
    res = {}
    key_order = ['node_type', 'title', 'type', 'default', 'env_vars', 'items', 'additionalProperties', 'format', 'visibility', 'description', 'example', 'tags', 'properties']
    for k in key_order:
        if k in obj:
            res[k] = obj[k]
    # validate the keys are the same
    missing = set(obj.keys()) - set(res.keys())
    if missing:
        raise RuntimeError('missing keys: %s' % (missing,))
    return res


def reorder_it(schema, currpath, trackorder):
    currkey = '.'.join(currpath)
    obj = schema['properties']
    res = {}
    useorder = trackorder[currkey]
    havekeys = obj.keys()
    missing_from_want = set(useorder) - set(havekeys)
    # This should always be empty! Because config_template keys should be a subset of the schema
    if len(missing_from_want):
        raise RuntimeError('*** key:%s missing: %s' % (currkey, missing_from_want))
    # If there are missing from `havekeys` that's okay. These are *undocumented* keys that are
    # defined in setup.go but not in the config_template.yaml
    missing_from_have = set(havekeys) - set(useorder)
    # Iterate the keys in order seen in config_template.yaml
    for k in useorder:
        item = obj[k]
        if item.get('node_type') == 'section':
            item = reorder_it(item, currpath + [k], trackorder)
        res[k] = nice_key_order(item)
    # Iterate the rest of the (undocumented) keys
    for k in missing_from_have:
        item = obj[k]
        if item.get('node_type') == 'section':
            item = reorder_it(item, currpath + [k], trackorder)
        res[k] = nice_key_order(item)
    schema['properties'] = res
    return schema


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("error usage")
        sys.exit(1)

    with open(sys.argv[1], "r") as f:
        template = f.read()

    with open(sys.argv[2], "r") as f:
        schema = yaml.safe_load(f)

    parser = Parser()
    parser.run(template, schema["properties"])

    # Use the `trackorder` collected from the config_template.yaml to reorder
    # the keys in the dict.
    schema = reorder_it(schema, [], parser.trackorder)

    # Output the first 10 elements to stdout, for debugging purposes
    for n,(k,v) in enumerate(schema['properties'].items()):
        print('%d => %s' % (n,k))
        if n >= 10:
            break

    with open(sys.argv[3], "w") as f:
        f.write(yaml.dump(schema, sort_keys=False))
