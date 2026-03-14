package com.example;

import android.os.IBinder;

parcelable GenericData {
    List<String> names;
    Map<String, int> values;
    @nullable List<IBinder> binders;
}
