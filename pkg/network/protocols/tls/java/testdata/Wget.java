// Need to be compiled with java7

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.URL;
import javax.net.ssl.HttpsURLConnection;

import javax.net.ssl.HostnameVerifier;
import javax.net.ssl.SSLSession;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;
import java.security.cert.X509Certificate;
import java.security.cert.CertificateException;
import java.security.NoSuchAlgorithmException;
import java.security.KeyManagementException;
import java.security.KeyStore;
import java.security.KeyStoreException;

public class Wget  {

    public static void main(String[] args) {
        URL url;
        boolean dumpContent = false;
        if (args.length == 0) {
            System.out.println("Wget <url>");
            System.exit(1);
        }

        try {
            System.out.println("waiting 11 seconds");
            // sleep 11 seconds before doing the request, as the process need to be injected
            Thread.sleep(11000);
        } catch (InterruptedException intException) {
            intException.printStackTrace();
            System.exit(1);
        }
        System.out.println("finished waiting");

        try {
            TrustManager[] trustAllCerts = new TrustManager[] {
                new X509TrustManager() {
                    public java.security.cert.X509Certificate[] getAcceptedIssuers() {
                        return null;
                    }
                    @Override
                    public void checkClientTrusted(X509Certificate[] arg0, String arg1)
                        throws CertificateException {}

                    @Override
                    public void checkServerTrusted(X509Certificate[] arg0, String arg1)
                        throws CertificateException {}
                }
            };

            KeyStore trustStore = KeyStore.getInstance(KeyStore.getDefaultType());
            trustStore.load(null, null);

            SSLContext sc=null;
            try {
                sc = SSLContext.getInstance("TLSv1.3");
            } catch (NoSuchAlgorithmException e) {
                e.printStackTrace();
            }
            try {
                sc.init(null, trustAllCerts, new java.security.SecureRandom());
            } catch (KeyManagementException e) {
                e.printStackTrace();
            }
            HttpsURLConnection.setDefaultSSLSocketFactory(sc.getSocketFactory());


            url = new URL(args[0]);

            HttpsURLConnection connection = (HttpsURLConnection) url.openConnection();
            connection.setRequestMethod("GET");
            connection.setConnectTimeout(15 * 1000);
            connection.setReadTimeout(15 * 1000);

            // skip certificate validation
            connection.setHostnameVerifier(new HostnameVerifier() {
                public boolean verify(String s, SSLSession sslSession) {
                    return true;
                }
            });
            System.out.println("Response code = " + connection.getResponseCode());

            BufferedReader br = new BufferedReader(new InputStreamReader(connection.getInputStream()));
            String input;
            while ((input = br.readLine()) != null) {
                if (dumpContent) {
                    System.out.println(input);
                }
            }
            connection.disconnect();
        } catch (Exception e) {
            e.printStackTrace();
            System.exit(1);
        }
    }
}
