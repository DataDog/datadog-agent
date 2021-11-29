const tracer = require("dd-trace");

const sleep = (ms) => {
  return new Promise((resolve) => setTimeout(resolve, ms));
};

exports.simpleTest = async function (event) {
  // submit a custom span
  await tracer.trace('integration-test', async () => {
    await sleep(100);
  });

  return {
    statusCode: 200,
    body: "ok",
  };
};
