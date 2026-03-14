package android.os;

interface IServiceManager {
    IBinder getService(String name);
    IBinder checkService(String name);
    void addService(String name, IBinder service, boolean allowIsolated, int dumpPriority);
    String[] listServices(int dumpPriority);
    boolean isDeclared(String name);
}
