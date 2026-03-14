package android.os;

interface IComplex {
    const int VERSION = 3;
    const int FLAG_A = 1;
    const int FLAG_B = 2;
    const int FLAG_ALL = 3;

    void processItems(int[] ids, String[] names);
    String[] getNames(int category);
    int[] getIds();
    boolean update(String key, int value);
}
