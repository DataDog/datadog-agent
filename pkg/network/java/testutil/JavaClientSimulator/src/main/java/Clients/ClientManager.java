package Clients;

import java.io.IOException;
import java.security.KeyManagementException;
import java.security.KeyStoreException;
import java.security.NoSuchAlgorithmException;

public class ClientManager {

    public enum ClientType{
        apache,
        okhttp,
        httpclient,
        urlconnection,
        unsupported,
    }

    private static ClientType getClientType(String clientTypeArg)  {
        try {
            return ClientType.valueOf(clientTypeArg.toLowerCase());
        } catch (IllegalArgumentException e) {
            return ClientType.unsupported;
        }
    }

    public static void executeCallback(String clientTypeArg, int iterations, long sleepInterval, String url) throws InterruptedException, NoSuchAlgorithmException, KeyStoreException, KeyManagementException, IOException {

        ClientType clientType = getClientType(clientTypeArg);
        JavaClients clients = new JavaClients();
        clients.init();

        System.out.println("Executing handler for " + clientType);
        System.out.println("URL: " + url);
        System.out.println("Iterations: " + iterations);
        System.out.println("Interval: " + sleepInterval);

        Runnable callback;

        // Execute handler based on client type
        switch (clientType) {
            case apache:

                callback = () -> {
                    try {
                        clients.HttpApacheClient(url);
                    } catch (IOException e) {
                        throw new RuntimeException(e);
                    }
                };
                break;
            case okhttp:
                callback = () -> {
                    try {
                        clients.OkHttpClient(url);
                    } catch (IOException e) {
                        throw new RuntimeException(e);
                    }
                };
                break;
            case httpclient:
                callback = () -> {
                    try {
                        clients.HTTPClient(url);
                    } catch (IOException e) {
                        throw new RuntimeException(e);
                    }
                };
                break;
            case urlconnection:
                callback = () -> {
                    try {
                        clients.HttpsURLConnection(url);
                    } catch (IOException e) {
                        throw new RuntimeException(e);
                    }
                };
                break;
            default:
                throw new IllegalArgumentException("Unsupported callback type: " + clientType);
        }
        executeCallbackLogic(iterations, sleepInterval, callback);
        clients.close();
    }

    private static void executeCallbackLogic(int iterations, long sleepInterval, Runnable callback) throws InterruptedException {
        if (iterations == -1) {
            // Infinite loop
            while (true) {
                callback.run();
                if (sleepInterval > 0)
                {
                    Thread.sleep(sleepInterval);
                }
            }
        } else {
            // Fixed number of iterations
            for (int i = 0; i < iterations; i++) {
                callback.run();
                if (sleepInterval > 0)
                {
                    Thread.sleep(sleepInterval);
                }
            }
        }
    }
}
