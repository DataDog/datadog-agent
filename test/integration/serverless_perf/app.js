exports.handler = async () => {
    const response = {
        statusCode: 200,
        body: JSON.stringify({
            msg : "hello world !"
        }),
    };
    return response;
};
