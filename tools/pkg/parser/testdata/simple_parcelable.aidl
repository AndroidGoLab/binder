package com.example;

parcelable SimpleData {
    int id;
    String name = "default";
    boolean active = true;
    long timestamp;
    @nullable String description;
    int[] scores;
}
