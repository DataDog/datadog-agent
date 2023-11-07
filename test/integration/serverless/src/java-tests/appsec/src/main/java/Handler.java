import java.util.Map;
import java.util.HashMap;

import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestHandler;
import com.amazonaws.services.lambda.runtime.events.*;

public class Handler implements RequestHandler<APIGatewayV2ProxyRequestEvent, APIGatewayV2ProxyResponseEvent> {
    public APIGatewayV2ProxyResponseEvent handleRequest(APIGatewayV2ProxyRequestEvent request, Context context) {
        final Map<String, String> headers = new HashMap<>();
        headers.put("Content-Encoding", "text/plain");

        final APIGatewayV2ProxyResponseEvent res = new APIGatewayV2ProxyResponseEvent();
        res.setStatusCode(200);
        res.setBody("ok");
        res.setHeaders(headers);

        return res;
    }
}
