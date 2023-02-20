// Need to be compiled with java7
 
import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.URL;
import javax.net.ssl.HttpsURLConnection;
 
public class Wget {
    public static void main(String[] args) {
        URL url;
        boolean dumpContent = false;
        if (args.length == 0) {
            System.out.println("Wget <url>");
            System.exit(1);
        }
        try {
            System.out.println("waiting 11 seconds");
            // sleep 10 seconds before doing the request, as the process need to be injected
            Thread.sleep(11000);

            url = new URL(args[0]);
            HttpsURLConnection connection = (HttpsURLConnection) url.openConnection();
            System.out.println("Response code = " + connection.getResponseCode());

            BufferedReader br = new BufferedReader(new InputStreamReader(connection.getInputStream()));
            String input;
            while ((input = br.readLine()) != null) {
                if (dumpContent) {
                    System.out.println(input);
                }
            }

        } catch (IOException urlException) {
            urlException.printStackTrace();
            System.exit(1);
        } catch (InterruptedException intException) {
            intException.printStackTrace();
            System.exit(1);
        }
    }
 
}
