# flake8: noqa
import logging
import time

logger = logging.getLogger(__name__)


def log(event, context):
    # Sleep to ensure correct log ordering
    time.sleep(0.25)
    logger.error("XXX LOG 0 XXX")
    time.sleep(0.25)
    logger.error("XXX LOG 1 XXX")
    time.sleep(0.25)
    logger.error("XXX LOG 2 XXX")
    time.sleep(0.25)
    return {"statusCode": 200, "body": "ok"}
