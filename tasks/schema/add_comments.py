#!/usr/bin/env python


def add_comments(schema, comments_info):
    for key, comment in comments_info.items():
        if not comment:
            continue
        node = schema
        for k in key.split("."):
            node = node["properties"].get(k)
            if not node:
                break
        if not node:
            continue
        node["comment"] = comment
    return schema
