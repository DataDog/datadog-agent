const { datadog, sendDistributionMetric } = require("datadog-lambda-js");

let shouldSendMetric = true;

async function myHandler(event, context) {
  if (shouldSendMetric) {
    sendDistributionMetric("serverless.lambda-extension.integration-test.count", 1);
    shouldSendMetric = false;
  }
  return {
    statusCode: 200,
    body: 'ok'
  };
}

async function myTimeoutHandler(event, context) {
  if (shouldSendMetric) {
    sendDistributionMetric("serverless.lambda-extension.integration-test.count", 1);
    shouldSendMetric = false;
  }
  await new Promise(r => setTimeout(r, 15 * 60 * 1000)); // max timeout value allowed by AWS
  invocationCount += 1;
  return {
    statusCode: 200,
    body: 'ok'
  };
}

async function myErrorHandler(event, context) {
  throw new Error("Something went wrong");
}

module.exports.enhancedMetricTest = datadog(myHandler);
module.exports.noEnhancedMetricTest = datadog(myHandler);
module.exports.timeoutMetricTest = datadog(myTimeoutHandler);
module.exports.errorTest = datadog(myErrorHandler);
