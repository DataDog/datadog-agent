/*
Need to be compiled with java7

javac TestAgentLoaded.java
jar cvmf META-INF/MANIFEST.MF TestAgentLoaded.jar TestAgentLoaded.class
 */

import java.lang.instrument.Instrumentation;
import java.io.FileOutputStream;

public class TestAgentLoaded {

    public static void agentmain(String agentArgs, Instrumentation inst) {
        try {
            // parsing the argument like agent-usm.jar
            if (agentArgs != ""){
                //split arguments by space character
                String[] args = agentArgs.split(" ");
                for (String arg : args){
                    //we only parse the arguments of the form "arg=value" (e.g: dd.debug.enabled=true)
                    String[] keyValTuple = arg.split("=");
                    if ((keyValTuple.length == 2) && (keyValTuple[0].equals("testfile"))) {
                        System.out.println("touch file "+keyValTuple[1]);
                        new FileOutputStream(keyValTuple[1]).close();
                    }
                }
            }
        } catch (Exception ex) {
            System.out.println(ex);
        } finally {
            System.out.println("loading TestAgentLoaded.agentmain("+agentArgs+")");
        }
    }
}
