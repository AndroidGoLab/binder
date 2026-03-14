package android.os;

interface IExample {
    const int VERSION = 1;
    const String DESCRIPTOR = "android.os.IExample";

    void doSomething();
    int getVersion();
}
