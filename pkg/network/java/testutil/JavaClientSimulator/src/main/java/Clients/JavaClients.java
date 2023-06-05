package Clients;

import okhttp3.Call;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import org.apache.commons.cli.ParseException;
import org.apache.http.client.methods.CloseableHttpResponse;
import org.apache.http.client.methods.HttpGet;
import org.apache.http.impl.client.CloseableHttpClient;
import org.apache.http.impl.client.HttpClients;

import javax.net.ssl.HttpsURLConnection;
import java.io.IOException;
import java.net.MalformedURLException;
import java.net.URI;
import java.net.URL;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

public class JavaClients {

    public enum ClientType{
        apache,
        okhttp,
        httpclient,
        urlconnection,
        unsupported,
    }

    private static ClientType getClientType(String clientTypeArg) throws ParseException {
        try {
            return ClientType.valueOf(clientTypeArg.toLowerCase());
        } catch (IllegalArgumentException e) {
            return ClientType.unsupported;
        }
    }

    public static void executeCallback(String clientTypeArg, int iterations, long sleepInterval, String url) throws InterruptedException, ParseException {

        ClientType clientType = getClientType(clientTypeArg);

        System.out.println("URL: " + url);
        System.out.println("Iterations: " + iterations);
        System.out.println("Interval: " + sleepInterval);

        Runnable callback = null;

        // Execute handler based on client type
        switch (clientType) {
            case apache:
                System.out.println("Executing handler for Apache Http client:");
                callback = () -> {
                    try {
                        JavaClients.HTTPApacheClientExample(url);
                    } catch (IOException e) {

                    }
                };
                executeCallbackLogic(iterations, sleepInterval, url, callback);
                break;
            case okhttp:
                System.out.println("Executing handler for OkHttp client:");
                callback = () -> {
                    try {
                        JavaClients.OkHttpClient(url);
                    } catch (IOException e) {

                    }
                };
                executeCallbackLogic(iterations, sleepInterval, url, callback);
                break;
            case httpclient:
                System.out.println("Executing handler for HttpClient client:");
                callback = () -> {
                    try {
                        JavaClients.HTTPClient(url);
                    } catch (IOException e) {

                    }
                };
                executeCallbackLogic(iterations, sleepInterval, url, callback);
                break;
            case urlconnection:
                System.out.println("Executing handler for URLConnection client:");
                callback = () -> {
                    try {
                        JavaClients.HttpsURLConnection(url);
                    } catch (IOException e) {

                    }
                };
                executeCallbackLogic(iterations, sleepInterval, url, callback);
                break;
            default:
                throw new IllegalArgumentException("Unsupported callback type: " + clientType);
        }
    }

    private static void executeCallbackLogic(int iterations, long sleepInterval, String url, Runnable callback) throws InterruptedException {
        if (iterations == -1) {
            // Infinite loop
            while (true) {
                callback.run();
                if (sleepInterval != 0)
                {
                    Thread.sleep(sleepInterval);
                }
            }
        } else {
            // Fixed number of iterations
            for (int i = 0; i < iterations; i++) {
                callback.run();
                if (sleepInterval != 0)
                {
                    Thread.sleep(sleepInterval);
                }
            }
        }
    }

    private static String URL_SCHEME = "https://";
    private static void HttpsURLConnection(String url) throws IOException {
        HttpsURLConnection urlConnection =(HttpsURLConnection) new URL(URL_SCHEME+url).openConnection();
        int res = urlConnection.getResponseCode();
        System.out.println("Response: " + res);
    }

    private static void OkHttpClient(String url) throws IOException {
        Request request = new Request.Builder()
                .url(URL_SCHEME+url)
                .build();
        OkHttpClient client = new OkHttpClient();
        Call call = client.newCall(request);
        Response response = call.execute();
        System.out.println("Response: " + response);
    }

    private static void HTTPApacheClientExample(String url) throws IOException {
        CloseableHttpClient httpClient = HttpClients.createDefault();
        try {
            HttpGet request = new HttpGet("https://"+url);
            CloseableHttpResponse response = httpClient.execute(request);
            System.out.println("Response: " + response);
        } catch (IOException e) {
            e.printStackTrace();
        }
        finally {
            httpClient.close();
        }
    }

    private static void HTTPClient(String url) throws IOException {
        try {
            HttpClient httpClient = HttpClient.newHttpClient();
            HttpRequest request = HttpRequest.newBuilder()
                    .uri(URI.create(URL_SCHEME+url))
                    .version(HttpClient.Version.HTTP_1_1)
                    .build();
            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
            System.out.println("Response " + response.toString());
        } catch (IOException  e) {
            e.printStackTrace();
        } catch (InterruptedException e) {
            throw new RuntimeException(e);
        }
    }
}
