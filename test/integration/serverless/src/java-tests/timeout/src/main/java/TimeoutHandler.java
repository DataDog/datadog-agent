import java.util.Map;
import java.util.LinkedHashMap;
import java.lang.Thread;

import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestHandler;
import com.amazonaws.services.lambda.runtime.events.APIGatewayV2ProxyRequestEvent;

public class TimeoutHandler implements RequestHandler<APIGatewayV2ProxyRequestEvent, Map<String, Object>>{
    public Map<String, Object> handleRequest(APIGatewayV2ProxyRequestEvent request, Context context){
        //trigger a timeout
        try {
            Thread.sleep(10000);
        } catch (InterruptedException e) {
            e.printStackTrace();
        }

        Map<String, Object> res = new LinkedHashMap();
        res.put("statusCode", 200);
        res.put("body", "ok");
        return res;
    }
}
