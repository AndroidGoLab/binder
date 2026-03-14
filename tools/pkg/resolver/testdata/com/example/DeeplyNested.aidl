package com.example;

parcelable DeeplyNested {
    int level1;

    parcelable Level2 {
        int level2;

        parcelable Level3 {
            int level3;
        }
    }
}
