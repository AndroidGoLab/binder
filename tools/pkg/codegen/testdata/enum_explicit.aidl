package android.os;

@Backing(type="int")
enum Priority {
    LOW = 0,
    NORMAL = 1,
    HIGH = 10,
    CRITICAL,
    URGENT = 100,
    MAXIMUM,
}
