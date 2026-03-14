package android.os;

oneway interface ICallback {
    void onResult(int code, String message);
}
