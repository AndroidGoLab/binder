package com.example;

import android.os.IBinder;

interface ISimpleService {
    const int VERSION = 1;

    void doSomething(in String name, int value);
    String getName();
    oneway void fireAndForget(in String message);
    @nullable IBinder getRemote();
    List<String> getItems(int offset, int count);
}
