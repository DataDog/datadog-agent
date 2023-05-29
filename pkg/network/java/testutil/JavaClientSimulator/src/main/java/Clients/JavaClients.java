package Clients;

import okhttp3.Call;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import org.apache.http.client.methods.CloseableHttpResponse;
import org.apache.http.client.methods.HttpGet;
import org.apache.http.impl.client.CloseableHttpClient;
import org.apache.http.impl.client.HttpClients;

import javax.net.ssl.HttpsURLConnection;
import java.io.IOException;
import java.net.URI;
import java.net.URL;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

public class JavaClients {

    private static String URL_SCHEME = "https://";
    public static void HttpsURLConnection(int iterations, String url) throws IOException, InterruptedException {
        if (iterations == -1){
            while (true){
                HttpsURLConnection urlConnection =(HttpsURLConnection) new URL(URL_SCHEME+url).openConnection();
                int res = urlConnection.getResponseCode();
                System.out.println("response: " + res);
                Thread.sleep(1000);
            }
        } else{
           while (iterations > 0) {
               HttpsURLConnection urlConnection =(HttpsURLConnection) new URL(URL_SCHEME+url).openConnection();
               int res = urlConnection.getResponseCode();
               System.out.println("response: " + res);
               iterations--;
           }
        }
    }

    public static void OkHttpClient(int iterations, String url) throws IOException, InterruptedException {
        Request request = new Request.Builder()
                .url(URL_SCHEME+url)
                .build();
        if (iterations == -1){
            while (true){
                OkHttpClient client = new OkHttpClient();
                Call call = client.newCall(request);
                Response response = call.execute();
                System.out.println("response: " + response);
                Thread.sleep(1000);
            }
        } else{
            while (iterations > 0) {
                OkHttpClient client = new OkHttpClient();
                Call call = client.newCall(request);
                Response response = call.execute();
                System.out.println("response: " + response);
                iterations--;
            }
        }
    }

    public static void HTTPApacheClientExample(int iterations, String url) throws InterruptedException, IOException {
        if (iterations == -1){
            while (true){
                CloseableHttpClient httpClient = HttpClients.createDefault();
                try {
                    HttpGet request = new HttpGet("https://"+url);
                    CloseableHttpResponse response = httpClient.execute(request);
                    System.out.println("response: " + response);
                } catch (IOException e) {
                    e.printStackTrace();
                }
                finally {
                    httpClient.close();
                    Thread.sleep(1000);
                }

            }
        } else{
            while (iterations > 0) {
                CloseableHttpClient httpClient = HttpClients.createDefault();
                try {
                    HttpGet request = new HttpGet("https://"+url);
                    CloseableHttpResponse response = httpClient.execute(request);
                    System.out.println("response: " + response);
                } catch (IOException e) {
                    e.printStackTrace();
                }
                finally {
                    httpClient.close();
                    iterations--;
                }
            }
        }
    }

    public static void HTTPClientExample(int iterations, String url) throws InterruptedException {
        if (iterations == -1){
            while (true){
                try {
                    HttpClient httpClient = HttpClient.newHttpClient();
                    HttpRequest request = HttpRequest.newBuilder()
                            .uri(URI.create(URL_SCHEME+url))
                            .version(HttpClient.Version.HTTP_1_1)
                            .build();
                    HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
                    System.out.println("Response " + response.toString());
                } catch (IOException | InterruptedException e) {
                    e.printStackTrace();
                }
                finally {
                    Thread.sleep(5000);
                }
            }
        } else{
            while (iterations > 0) {
                try {
                    HttpClient httpClient = HttpClient.newHttpClient();
                    HttpRequest request = HttpRequest.newBuilder()
                            .uri(URI.create(URL_SCHEME+url))
                            .version(HttpClient.Version.HTTP_1_1)
                            .build();
                    HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
                    System.out.println("Response " + response.toString());
                } catch (IOException | InterruptedException e) {
                    e.printStackTrace();
                }
                finally {
                    iterations--;
                }
            }
        }
    }
}
