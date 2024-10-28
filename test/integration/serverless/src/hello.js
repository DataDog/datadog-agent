exports.lambda_handler = async function (event) {
    console.log("Logging from hello.js");
    return {
        statusCode: 200,
        body: "ok",
    };
};
