const { datadog } = require("datadog-lambda-js");

async function myHandler(event, context) {
  console.error("XXX LOG 0 XXX");
  console.error("XXX LOG 1 XXX");
  console.error("XXX LOG 2 XXX");
  return {
    statusCode: 200,
    body: 'ok'
  };
}

module.exports.logTest = datadog(myHandler);