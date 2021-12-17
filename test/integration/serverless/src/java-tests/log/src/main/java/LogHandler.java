
import java.util.Map;
import java.util.LinkedHashMap;

import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestHandler;
import com.amazonaws.services.lambda.runtime.events.APIGatewayV2ProxyRequestEvent;
import com.datadoghq.datadog_lambda_java.DDLambda;

public class LogHandler implements RequestHandler<APIGatewayV2ProxyRequestEvent, Map<String, Object>>{
    public Map<String, Object> handleRequest(APIGatewayV2ProxyRequestEvent request, Context context){
        DDLambda ddl = new DDLambda(context); //Required to initialize the trace
		Map<String, Object> res = new LinkedHashMap();
		res.put("statusCode", 200);
		res.put("body", "ok");

		System.out.println("XXX Log 0 XXX");
		System.out.println("XXX Log 1 XXX");
		System.out.println("XXX Log 2 XXX");

		ddl.finish();
		return res;
    }
}
