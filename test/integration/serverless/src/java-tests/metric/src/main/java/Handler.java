import java.util.Map;
import java.util.LinkedHashMap;

import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestHandler;
import com.amazonaws.services.lambda.runtime.events.*;

public class Handler implements RequestHandler<APIGatewayV2ProxyRequestEvent, Map<String, Object>>{
    public Map<String, Object> handleRequest(APIGatewayV2ProxyRequestEvent request, Context context){
        Map<String, Object> res = new LinkedHashMap();
        res.put("statusCode", 200);
        res.put("body", "ok");
        return res;
    }
}
