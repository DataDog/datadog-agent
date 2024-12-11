# flake8: noqa


def handler(_event, _context):
    return {"statusCode": 200, "body": "ok", "headers": {"Content-Type": "text/plain"}}
