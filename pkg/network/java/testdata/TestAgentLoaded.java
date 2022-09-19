/*
javac TestAgentLoaded.java
jar cvmf META-INF/MANIFEST.MF TestAgentLoaded.jar TestAgentLoaded.class
 */

import java.lang.instrument.Instrumentation;
import java.io.FileOutputStream;

public class TestAgentLoaded {

    public static void agentmain(String agentArgs, Instrumentation inst) {
        System.out.println("loading TestAgentLoaded.agentmain("+agentArgs+")");
        try {
            new FileOutputStream(agentArgs).close();
        } catch (Exception ex) {
            System.out.println(ex);
        }
    }
    
}
