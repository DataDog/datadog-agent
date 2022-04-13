
import java.util.Map;
import java.util.LinkedHashMap;
import java.lang.Error;

import com.amazonaws.services.lambda.runtime.Context;
import com.amazonaws.services.lambda.runtime.RequestHandler;
import com.amazonaws.services.lambda.runtime.events.APIGatewayV2ProxyRequestEvent;
import com.datadoghq.datadog_lambda_java.DDLambda;

public class ErrorHandler implements RequestHandler<APIGatewayV2ProxyRequestEvent, Map<String, Object>>{
    public Map<String, Object> handleRequest(APIGatewayV2ProxyRequestEvent request, Context context){
		throw new Error("oops");
    }
}
