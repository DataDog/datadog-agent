import json
from dataclasses import dataclass

import tasks.libs.cws.common as common


@dataclass
class EventTypeProperty:
    name: str
    definition: str
    doc_link: str  # noqa: F841


@dataclass
class EventType:
    name: str
    kind: str
    definition: str
    min_agent_version: str  # noqa: F841
    experimental: bool
    properties: list[EventTypeProperty]


@dataclass
class Example:
    expression: str  # noqa: F841
    description: str


@dataclass
class PropertyDocumentation:
    name: str
    link: str
    datatype: str  # noqa: F841
    definition: str
    prefixes: list[str]  # noqa: F841
    constants: str
    constants_link: str  # noqa: F841
    examples: list[Example]


@dataclass
class Constant:
    name: str
    architecture: str


@dataclass
class Constants:
    name: str
    link: str
    definition: str
    all: list[Constant]


def build_event_types(top_node):
    output = []
    for et in top_node["event_types"]:
        event_type = EventType(
            et["name"], et["type"], et["definition"], et["from_agent_version"], et["experimental"], []
        )
        for p in et["properties"]:
            prop = EventTypeProperty(p["name"], p["definition"], p["property_doc_link"])
            event_type.properties.append(prop)
        output.append(event_type)
    return output


def build_properties_doc(top_node):
    output = []
    for property in top_node["properties_doc"]:
        property_doc = PropertyDocumentation(
            property["name"],
            property["link"],
            property["type"],
            property["definition"],
            property["prefixes"],
            property["constants"],
            property["constants_link"],
            [],
        )
        for exp in property["examples"]:
            property_doc.examples.append(Example(exp["expression"], exp["description"]))
        output.append(property_doc)
    return output


def build_constants(top_node):
    output = []
    for cs in top_node["constants"]:
        constants = Constants(cs["name"], cs["link"], cs["description"], [])
        for c in cs["all"]:
            constants.all.append(Constant(c["name"], c["architecture"]))
        output.append(constants)
    return output


def generate_secl_documentation(input: str, output: str, template: str):
    secl_json_file = open(input)
    json_top_node = json.load(secl_json_file)
    secl_json_file.close()

    event_types = build_event_types(json_top_node)
    properties_doc_list = build_properties_doc(json_top_node)
    constants_list = build_constants(json_top_node)

    with open(output, "w") as output_file:
        print(
            common.fill_template(
                template,
                event_types=event_types,
                constants_list=constants_list,
                properties_doc_list=properties_doc_list,
            ),
            file=output_file,
        )
