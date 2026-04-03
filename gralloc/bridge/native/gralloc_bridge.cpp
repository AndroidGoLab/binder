// gralloc_bridge.cpp — Minimal C wrapper around HIDL IMapper v3.0
// for importing and locking gralloc buffers from Go.
//
// The ranchu mapper's lockYCbCr calls readFromHost internally to pull
// pixel data from the host GPU into the locked buffer. This bridge
// exists because goldfish emulator buffers use host GPU memory that
// cannot be read via direct mmap — the IMapper is the only path.
//
// Compiled with the Android NDK (-static-libstdc++ to avoid
// libc++_shared.so dependency) and pushed alongside the test binary.
// Loaded at runtime via purego/dlopen.

#include <cstring>
#include <dlfcn.h>
#include <android/hardware/graphics/mapper/3.0/IMapper.h>
#include <cutils/native_handle.h>

using android::hardware::graphics::mapper::V3_0::IMapper;
using android::hardware::graphics::mapper::V3_0::Error;
using android::hardware::graphics::mapper::V3_0::YCbCrLayout;
using android::hardware::hidl_handle;
using android::sp;

static sp<IMapper> sMapper;

extern "C" {

// bridge_init loads the HIDL IMapper. Returns 0 on success.
int bridge_init() {
    if (sMapper != nullptr) return 0;

    // Try HIDL_FETCH_IMapper from the ranchu mapper .so.
    void* lib = dlopen(
        "android.hardware.graphics.mapper@3.0-impl-ranchu.so",
        RTLD_LAZY);
    if (!lib) {
        lib = dlopen(
            "/vendor/lib64/hw/android.hardware.graphics.mapper@3.0-impl-ranchu.so",
            RTLD_LAZY);
    }
    if (!lib) return -1;

    auto fetch = reinterpret_cast<IMapper* (*)(const char*)>(
        dlsym(lib, "HIDL_FETCH_IMapper"));
    if (!fetch) return -2;

    sMapper = fetch("default");
    if (sMapper == nullptr) return -3;

    return 0;
}

// bridge_import imports a raw gralloc buffer handle.
// fds/ints are the NativeHandle data. Returns an opaque buffer pointer
// (for use with bridge_lock), or NULL on failure.
void* bridge_import(const int* fds, int num_fds,
                    const int* ints, int num_ints) {
    if (sMapper == nullptr) return nullptr;

    native_handle_t* nh = native_handle_create(num_fds, num_ints);
    if (!nh) return nullptr;
    memcpy(&nh->data[0], fds, num_fds * sizeof(int));
    memcpy(&nh->data[num_fds], ints, num_ints * sizeof(int));

    hidl_handle rawHandle(nh);
    void* result = nullptr;

    sMapper->importBuffer(rawHandle, [&](Error err, void* buf) {
        if (err == Error::NONE) {
            result = buf;
        }
    });

    native_handle_delete(nh);
    return result;
}

// bridge_lock_ycbcr locks a buffer for CPU read and returns Y/Cb/Cr
// plane pointers and strides. The ranchu mapper's lockYCbCr internally
// calls readFromHost (rcColorBufferCacheFlush + rcReadColorBufferYUV)
// to transfer pixel data from the host GPU. Returns 0 on success.
int bridge_lock_ycbcr(void* buffer, int width, int height,
                      void** out_y, void** out_cb, void** out_cr,
                      uint32_t* out_ystride, uint32_t* out_cstride,
                      uint32_t* out_chroma_step) {
    if (sMapper == nullptr || buffer == nullptr) return -1;

    IMapper::Rect region{0, 0, width, height};
    hidl_handle fence; // no fence

    int ret = -1;
    sMapper->lockYCbCr(buffer,
        0x3 /* CPU_READ_OFTEN */,
        region, fence,
        [&](Error err, const YCbCrLayout& layout) {
            if (err == Error::NONE) {
                *out_y = layout.y;
                *out_cb = layout.cb;
                *out_cr = layout.cr;
                *out_ystride = layout.yStride;
                *out_cstride = layout.cStride;
                *out_chroma_step = layout.chromaStep;
                ret = 0;
            }
        });
    return ret;
}

// bridge_lock locks a buffer for CPU read and returns a pointer to the
// pixel data. Works for any format (RGBA, RGB, etc.). Returns 0 on success.
int bridge_lock(void* buffer, int width, int height,
                void** out_data) {
    if (sMapper == nullptr || buffer == nullptr) return -1;

    IMapper::Rect region{0, 0, width, height};
    hidl_handle fence;

    int ret = -1;
    sMapper->lock(buffer,
        0x3 /* CPU_READ_OFTEN */,
        region, fence,
        [&](Error err, void* data, int32_t, int32_t) {
            if (err == Error::NONE) {
                *out_data = data;
                ret = 0;
            }
        });
    return ret;
}

// bridge_unlock unlocks a previously locked buffer.
void bridge_unlock(void* buffer) {
    if (sMapper == nullptr || buffer == nullptr) return;
    sMapper->unlock(buffer, [](Error, const hidl_handle&) {});
}

// bridge_free frees a previously imported buffer.
void bridge_free(void* buffer) {
    if (sMapper == nullptr || buffer == nullptr) return;
    sMapper->freeBuffer(buffer);
}

} // extern "C"
