// Need to be compiled with java7

import java.lang.Thread;
import java.lang.management.ManagementFactory;

class JustWait {
    public static void main(String[] args) {
        String pid = ManagementFactory.getRuntimeMXBean().getName();
        System.out.println("JustWait pid "+ pid);
        try {
            for(;;) {
                Thread.sleep(1000);
            }
        } catch (Exception ex) {
            System.out.println(ex);
        }
        System.out.println("JustWait pid "+pid+" ended");
    }
}
