package Clients;
import org.apache.commons.cli.*;

public class Main {

    //in milliseconds
    private static final int DEFAULT_TIMEOUT = 1000;

    private static void printHelp(Options options){
        HelpFormatter formatter = new HelpFormatter();
        formatter.printHelp("java -jar JavaClients client=<client> url=<url>\n",
                "",
                options,
                "\nprovide the url WITHOUT the protocol scheme (always using https)");
    }

    public static void main(String[] args) throws Exception {
        Options options = new Options();
        options.addRequiredOption("c", "client", true,
                "Client type: apache ; okhttp ; urlconnection ; httpclient");
        options.addRequiredOption("u", "url", true, "Target URL");
        Option iterationOption = Option.builder("i")
                .longOpt("iterations")
                .hasArg()
                .desc("Number of iterations. The default is infinitely")
                .required(false)
                .build();
        iterationOption.setType(Number.class);
        options.addOption(iterationOption);

        Option timeoutOption = Option.builder("t")
                .longOpt("timeout")
                .hasArg()
                .desc("Timeout between each call in ms. Default is 1 second, use 0 to send the requests without a timeout")
                .required(false)
                .build();
        timeoutOption.setType(Number.class);
        options.addOption(timeoutOption);

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
            if (cmd.hasOption("i")) {
                iterationsValue = ((Number)cmd.getParsedOptionValue("i")).intValue();
            }

            int interval = DEFAULT_TIMEOUT;
            if (cmd.hasOption("t")) {
                interval = ((Number)cmd.getParsedOptionValue("t")).intValue();
            }

            // Execute the appropriate handler based on client type
            ClientManager.executeCallback(clientTypeArg, iterationsValue, interval, url);
        } catch (ParseException e) {
            System.err.println("Error parsing command-line arguments: " + e.getMessage());
            printHelp(options);
        } catch (NumberFormatException e) {
            System.err.println("Error parsing iterations argument: " + e.getMessage());
            printHelp(options);
        }
    }
}
