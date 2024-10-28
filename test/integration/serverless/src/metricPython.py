# flake8: noqa
import time

from datadog_lambda.metric import lambda_metric

should_send_metric = True


def metric(event, context):
    global should_send_metric
    if should_send_metric:
        lambda_metric(metric_name='serverless.lambda-extension.integration-test.count', value=1)
        should_send_metric = False
    return {"statusCode": 200, "body": "ok"}


def timeout(event, context):
    global should_send_metric
    if should_send_metric:
        lambda_metric(metric_name='serverless.lambda-extension.integration-test.count', value=1)
        should_send_metric = False
    time.sleep(15 * 60)
    return {"statusCode": 200, "body": "ok"}


def error(event, context):
    raise Exception
