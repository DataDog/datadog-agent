async function noop(event, context) {
  return {
    statusCode: 200,
    body: 'ok'
  };
}

async function log(event, context) {
  // Sleep to ensure correct log ordering
  await sleep();
  console.error("XXX LOG 0 XXX");
  await sleep();
  console.error("XXX LOG 1 XXX");
  await sleep();
  console.error("XXX LOG 2 XXX");
  await sleep();
  return {
    statusCode: 200,
    body: 'ok'
  };
}

async function timeout(event, context) {
  await new Promise(r => setTimeout(r, 15 * 60 * 1000)); // max timeout value allowed by AWS
  return {
    statusCode: 200,
    body: 'ok'
  };
}

async function error(event, context) {
  throw new Error("Something went wrong");
}

function sleep() {
  return new Promise(resolve => setTimeout(resolve, 250));
}

module.exports.noop = noop;
module.exports.log = log;
module.exports.timeout = timeout;
module.exports.error = error;

