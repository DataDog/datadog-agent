public class DummyApp {
    public static void main(String[] args) {
        try {
            // Attempt to detect if Datadog Tracer is loaded by the javaagent
            Class<?> tracerClass = Class.forName("datadog.trace.api.GlobalTracer");

            System.out.println("Datadog Tracer is available.");

            try {
                // Try to get the version, if available
                Class<?> versionClass = Class.forName("datadog.trace.api.Version");
                String version = (String) versionClass.getDeclaredField("VERSION").get(null);

                System.out.println("Datadog Tracer version: " + version);
            } catch (ClassNotFoundException e) {
                System.out.println("Datadog Version class not found. Tracer is present but version info is unavailable.");
            }

        } catch (ClassNotFoundException e) {
            System.out.println("Datadog Tracer is NOT available.");
        } catch (Exception e) {
            System.out.println("Unexpected error: " + e.getMessage());
        }
    }
}