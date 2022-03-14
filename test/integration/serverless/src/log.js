const { datadog } = require("datadog-lambda-js");

async function myHandler(event, context) {
  // Sleep to ensure correct log ordering
  await sleep();
  console.error("XXX LOG 0 XXX");
  await sleep();
  console.error("XXX LOG 1 XXX");
  await sleep();
  console.error("XXX LOG 2 XXX");
  await sleep();
  console.error("XXX OBFUSCATED LOG M12345M XXX")

  return {
    statusCode: 200,
    body: 'ok'
  };
}

function sleep() {
  return new Promise(resolve => setTimeout(resolve, 250));
}

module.exports.logTest = datadog(myHandler);