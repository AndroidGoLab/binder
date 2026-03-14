package com.example;

parcelable ParcelWithNestedInterface {
    int value;

    interface IInner {
        void doSomething();
    }
}
