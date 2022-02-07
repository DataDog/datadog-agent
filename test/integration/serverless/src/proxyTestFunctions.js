async function noop(event, context) {
  return {
    statusCode: 200,
    body: 'ok'
  };
}

async function log(event, context) {
    console.error("XXX LOG 0 XXX");
    console.error("XXX LOG 1 XXX");
    console.error("XXX LOG 2 XXX");
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

module.exports.noop = noop;
module.exports.log = log;

