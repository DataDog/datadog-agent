# flake8: noqa
import logging

logger = logging.getLogger(__name__)


def log(event, context):
    logger.error("XXX LOG 0 XXX")
    logger.error("XXX LOG 1 XXX")
    logger.error("XXX LOG 2 XXX")
    return {"statusCode": 200, "body": "ok"}
