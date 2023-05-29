package Clients;
import org.apache.commons.cli.*;

import java.io.IOException;

public class Main {

    public enum ClientType{
        apache,
        okhttp,
        httpclient,
        urlconnection,
        unsupported,
    }


    private static void printHelp(Options options){
        HelpFormatter formatter = new HelpFormatter();
        formatter.printHelp("java -jar JavaClients client=<client> url=<url>\n",
                "",
                options,
                "\nprovide the url WITHOUT the protocol scheme (always using https)");
    }

    private static ClientType validateClientType(String clientTypeArg) throws ParseException {
        try {
            return ClientType.valueOf(clientTypeArg.toLowerCase());
        } catch (IllegalArgumentException e) {
            return ClientType.unsupported;
        }
    }
    private static void executeHandler(ClientType clientType, String url, int iterations) throws InterruptedException, IOException {
        // Execute handler based on client type
        System.out.println("URL: " + url);
        System.out.println("Iterations: " + iterations);
        switch (clientType) {
            case apache:

                System.out.println("Executing handler for Apache Http client:");
                JavaClients.HTTPApacheClientExample(iterations,url);
                break;
            case okhttp:
                System.out.println("Executing handler for OkHttp client:");
                JavaClients.OkHttpClient(iterations,url);
                break;
            case httpclient:
                System.out.println("Executing handler for HttpClient client:");
                JavaClients.HTTPClientExample(iterations,url);
                break;
            case urlconnection:
                System.out.println("Executing handler for URLConnection client:");
                JavaClients.HttpsURLConnection(iterations,url);
                break;
            default:
                System.out.println("Unsupported client");
        }
    }

    public static void main(String[] args) throws Exception {
        Options options = new Options();
        options.addRequiredOption("c", "client", true,
                "Client type: apache ; okhttp ; urlconnection ; httpclient");
        options.addRequiredOption("u", "url", true, "Target URL");
        Option iterationOption = Option.builder("i")
                .longOpt("iterations")
                .hasArg()
                .desc("Number of iterations")
                .required(false)
                .build();
        iterationOption.setType(Number.class);
        options.addOption(iterationOption);
        if (args.length == 0){
            printHelp(options);
            return;
        }
        // Parse command-line arguments
        CommandLineParser parser = new DefaultParser();
        try {
            CommandLine cmd = parser.parse(options, args);

            // Get arguments
            String clientTypeArg = cmd.getOptionValue("c");
            String url = cmd.getOptionValue("u");
            int iterationsValue = -1;
            if (cmd.hasOption("iterations")) {
                iterationsValue = ((Number)cmd.getParsedOptionValue("i")).intValue();
            }

            // Validate and convert arguments
            ClientType clientType = validateClientType(clientTypeArg);

            // Execute the appropriate handler based on client type
            executeHandler(clientType, url, iterationsValue);
        } catch (ParseException e) {
            System.err.println("Error parsing command-line arguments: " + e.getMessage());
            printHelp(options);
        } catch (NumberFormatException e) {
            System.err.println("Error parsing iterations argument: " + e.getMessage());
            printHelp(options);
        }


    }
}
