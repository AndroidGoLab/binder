package com.example;

parcelable ParcelWithNestedParcelable {
    int id;

    parcelable InnerData {
        String name;
    }
}
