import difflib
import os

from invoke.exceptions import Exit
from invoke.tasks import task

try:
    from termcolor import colored
except ImportError:

    def colored(*args):  # type: ignore
        return args[0]


@task(
    iterable="replacements",
    help={
        "xslt": "XSLT file to use for transformation",
        "replacements": "Override replacements to use for transformation. Multiple arguments of the form key=value",
        "xml": "File to use as a base for transformation. This can be one of the names in the tasks/microvm-sample-xmls directory (e.g., domain, network, volume) or a path to an XML file.",
    },
)
def check_xslt(_, xslt, replacements=None, xml="domain"):
    """
    Checks the XSLT transformations in the scenarios/aws/microVMs/microvms/resources path
    Useful for testing and checking transformation there without running the full pulumi stack
    """
    try:
        import lxml.etree as etree
    except ImportError:
        raise Exit("lxml is not installed. Please install it with `pip install lxml`") from None

    if os.path.exists(xml):
        xml_path = xml
    elif os.path.exists(f"tasks/microvm-sample-xmls/{xml}.xml"):
        xml_path = f"tasks/microvm-sample-xmls/{xml}.xml"
    elif os.path.exists(f"tasks/microvm-sample-xmls/{xml}"):
        xml_path = f"tasks/microvm-sample-xmls/{xml}"
    else:
        raise Exit(f"Could not find XML file {xml}")

    with open(xml_path) as f:
        base_xml = f.read()

    parser = etree.XMLParser(remove_blank_text=True)
    dom = etree.fromstring(base_xml, parser)

    default_replacements = {
        "sharedFSMount": "/opt/kernel-version-testing",
        "domainID": "local-ddvm-local-ubuntu_22.04-distro_local-ddvm-4-8192",
        "mac": "52:54:00:00:00:00",
        "nvram": "/tmp/nvram",
        "efi": "/tmp/efi",
        "vcpu": "4",
        "cputune": "<cputune></cputune>",
        "hypervisor": "hvf",
        "commandLine": "<arg value=\"test\"/>",
    }

    for repl in replacements or []:
        key, value = repl.split("=")
        default_replacements[key] = value

    with open(xslt) as f:
        data = f.read()

    for key, value in default_replacements.items():
        data = data.replace(f"{{{key}}}", value)

    xslt = etree.fromstring(data, parser)
    transform = etree.XSLT(xslt)
    newdom = transform(dom)

    orig_xml = etree.tostring(dom, pretty_print=True).decode('utf-8').replace('\\n', '\n')
    new_xml = etree.tostring(newdom, pretty_print=True).decode('utf-8').replace('\\n', '\n')

    print(colored("=== Original XML ===", "white"))
    print(orig_xml)
    print(colored("=== Transformed XML ===", "white"))
    print(new_xml)

    diff = difflib.unified_diff(orig_xml.split('\n'), new_xml.split('\n'), fromfile="original", tofile="transformed")

    print(colored("=== Diff ===", "white"))

    for line in diff:
        line = line.rstrip('\n')

        if line.startswith('-'):
            print(colored(line, "red"))
        elif line.startswith('+'):
            print(colored(line, "green"))
        elif line.startswith('@@'):
            print(colored(line, "blue"))
        else:
            print(line)
