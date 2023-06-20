package Clients;

import okhttp3.Call;
import okhttp3.OkHttpClient;
import okhttp3.Protocol;
import okhttp3.Request;
import okhttp3.Response;
import org.apache.http.client.methods.CloseableHttpResponse;
import org.apache.http.client.methods.HttpGet;
import org.apache.http.conn.ssl.NoopHostnameVerifier;
import org.apache.http.impl.client.CloseableHttpClient;
import org.apache.http.impl.client.HttpClients;

import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;
import java.io.Closeable;
import java.io.IOException;
import java.net.URI;
import java.net.URL;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.security.KeyManagementException;
import java.security.KeyStoreException;
import java.security.NoSuchAlgorithmException;
import java.security.cert.CertificateException;
import java.security.cert.X509Certificate;
import java.util.Arrays;

public class JavaClients implements Closeable {

    // Create a custom TrustManager that accepts all certificates
    // Create a TrustManager that accepts all certificates
    static X509TrustManager trustManager = new X509TrustManager() {
        @Override
        public void checkClientTrusted(X509Certificate[] x509Certificates, String s) throws CertificateException {
            // No validation needed, accepting all client certificates
        }

        @Override
        public void checkServerTrusted(X509Certificate[] x509Certificates, String s) throws CertificateException {
            // No validation needed, accepting all server certificates
        }

        @Override
        public X509Certificate[] getAcceptedIssuers() {
            // Return an empty array to accept all issuers
            return new X509Certificate[0];
        }
    };
    private static final TrustManager[] trustAllCerts = new TrustManager[]{trustManager};

    private static final String URL_SCHEME = "https://";

    private CloseableHttpClient apacheClient;
    private OkHttpClient okHttpClient;
    private HttpClient httpClient;

    public void init() throws NoSuchAlgorithmException, KeyManagementException, KeyStoreException {
        // Create a custom SSLContext to trust all certificates
        SSLContext sslContext = SSLContext.getInstance("TLS");
        sslContext.init(null, trustAllCerts, new java.security.SecureRandom());

        //configure HttpsURLConnection to trust all certificates and ignore host validation
        //URLConnection client will be recreated for each request
        HttpsURLConnection.setDefaultSSLSocketFactory(sslContext.getSocketFactory());
        HttpsURLConnection.setDefaultHostnameVerifier(NoopHostnameVerifier.INSTANCE);

        //create apache client once and configure it to trust all certificates and ignore host validation
        apacheClient = HttpClients.custom()
                .setSSLContext(sslContext)
                .setSSLHostnameVerifier(NoopHostnameVerifier.INSTANCE)
                .build();

        //create http client once and configure it to trust all certificates and ignore host validation
        httpClient = HttpClient.newBuilder().sslContext(sslContext).build();

        //create okhttp client once and configure it to trust all certificates and ignore host validation
        okHttpClient = new OkHttpClient.Builder()
                .hostnameVerifier(NoopHostnameVerifier.INSTANCE)
                .sslSocketFactory(sslContext.getSocketFactory(),(X509TrustManager) trustAllCerts[0])
                //by default okhttp is using http2.0
                .protocols(Arrays.asList(Protocol.HTTP_1_1))
                .build();
    }

    public void HttpsURLConnection(String url) throws IOException {
        HttpsURLConnection urlConnection =(HttpsURLConnection) new URL(URL_SCHEME+url).openConnection();
        String response = urlConnection.getResponseMessage();
        int  code = urlConnection.getResponseCode();
        System.out.println("Response code:  " + code + " ; Message: " + response);
    }

    public void OkHttpClient(String url) throws IOException {
        Request request = new Request.Builder()
                .url(URL_SCHEME+url)
                .build();
        Call call = okHttpClient.newCall(request);
        Response response = call.execute();
        System.out.println("Response: " + response);
    }

    public void HttpApacheClient(String url) throws IOException {
        HttpGet request = new HttpGet("https://"+url);
        try {
            CloseableHttpResponse response = apacheClient.execute(request);
            System.out.println("Response: " + response);
        } catch (IOException e) {
            e.printStackTrace();
        }
        finally {
            // TODO: in the future we should support re-using the same connection for apache client,
            // currently we are hitting the internal connection pool limit of the apacheclient,
            // since we create a new request object for the same route, which in turn tries to use a new connection
            // (the default connection limit of the apacheclient for the same route is 2
            request.releaseConnection();
        }
    }

    public void HTTPClient(String url) throws IOException {
        try {
            HttpRequest request = HttpRequest.newBuilder()
                    .uri(URI.create(URL_SCHEME+url))
                    //by default HttpCLient is using http 2.0
                    .version(HttpClient.Version.HTTP_1_1)
                    .build();
            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
            System.out.println("Response " + response.toString());
        } catch (IOException | InterruptedException e) {
            e.printStackTrace();
        }
    }

    @Override
    public void close() throws IOException {
        apacheClient.close();
        okHttpClient.dispatcher().executorService().shutdown();
        okHttpClient.connectionPool().evictAll();
    }
}
