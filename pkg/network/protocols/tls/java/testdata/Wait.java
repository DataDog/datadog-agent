// Need to be compiled with java7

import java.lang.Thread;
import java.lang.management.ManagementFactory;

class Wait {
    private static String getProcessId(final String fallback) {
        // Note: may fail in some JVM implementations
        // therefore fallback has to be provided

        // something like '<pid>@<hostname>', at least in SUN / Oracle JVMs
        final String jvmName = ManagementFactory.getRuntimeMXBean().getName();
        final int index = jvmName.indexOf('@');

        if (index < 1) {
            // part before '@' empty (index = 0) / '@' not found (index = -1)
            return fallback;
        }

        try {
            return Long.toString(Long.parseLong(jvmName.substring(0, index)));
        } catch (NumberFormatException e) {
            // ignore
        }
        return fallback;
    }

    public static void main(String[] args) {
        String progName = args[0];
        String pid = getProcessId("<PID>");
        System.out.println(progName + " pid "+ pid);
        try {
            for(;;) {
                Thread.sleep(1000);
            }
        } catch (Exception ex) {
            System.out.println(ex);
        }
        System.out.println(progName + " pid "+pid+" ended");
    }
}
