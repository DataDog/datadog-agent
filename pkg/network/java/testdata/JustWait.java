import java.lang.Thread;
import java.lang.ProcessHandle;

class JustWait {
    public static void main(String[] args) {
        long pid = ProcessHandle.current().pid();
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
