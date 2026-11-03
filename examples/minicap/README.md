# Minicap example

This example captures a virtual display and serves minicap-compatible JPEG
frames on an Android abstract Unix socket. It also supports raw RGBA output.
Running it requires access to SurfaceFlinger and the graphics allocator. Some
devices grant this access to the ADB shell; others require a privileged account.

```sh
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -o minicap ./examples/minicap
adb push minicap /data/local/tmp/minicap
adb shell /data/local/tmp/minicap -P 1080x2400@360x800/0 -n minicap
```

Use the physical display dimensions for the first pair in `-P` and the maximum
output dimensions for the second. The last value is 0, 90, 180, or 270. The
output is fitted to the display's aspect ratio. Run `minicap -h` for the
options, including JPEG quality (`-Q` or `-q`), frame skipping (`-S`), frame
rate (`-r`), display information (`-i`), a capture test (`-t`), and a single
JPEG screenshot to stdout (`-s`). The default display is 0. Secondary-display
capture requires an available layer-stack mapping; Android 10 does not expose
that mapping through the supported native interface.

For stdout modes invoked through `adb exec-out`, redirect the remote command's
stderr so the startup PID message is not mixed with the output:

```sh
adb exec-out sh -c '/data/local/tmp/minicap -P 1080x2400@360x800/0 -t 2>/dev/null'
adb exec-out sh -c '/data/local/tmp/minicap -P 1080x2400@360x800/0 -s 2>/dev/null' > screenshot.jpg
```

On Android 10, the virtual display requests secure composition, matching native
minicap; Android 14 uses a non-secure virtual display. Secure layers may be
omitted or blacked out, and protected graphics buffers are unsupported.

Forward the abstract socket to a local TCP port:

```sh
adb forward tcp:1313 localabstract:minicap
```

Each connection starts with a 24-byte minicap v1 banner: version and banner
length bytes, followed by little-endian 32-bit process ID, physical width,
physical height, virtual width, and virtual height, then orientation in
quarter turns and a quirks byte. Subsequent frames contain a little-endian
32-bit JPEG byte length followed by that many JPEG bytes. The server accepts a
new client after the previous client disconnects.

Frames are delivered when the display producer queues updated buffers; this is
not a per-view change event, and a static screen may not produce new frames.
`-r` caps the output rate, while `-S` skips older queued frames in favor of the
latest available frame.

For unframed, tightly packed RGBA8888 frames on stdout, use `-raw`:

```sh
adb exec-out sh -c '/data/local/tmp/minicap -P 1080x2400@360x800/0 -raw 2>/dev/null' > frames.rgba
```

Each raw frame is `output_width * output_height * 4` bytes. For 90- or
270-degree projection, the output width and height are exchanged. Terminate
capture with SIGINT or SIGTERM. Since raw output has no framing, read fixed-size
frame chunks and discard any incomplete trailing bytes if capture is interrupted
mid-frame.

On Android arm64, the native AHardwareBuffer allocator is used on API 29 and
later, without assuming a vendor-specific buffer layout. Capture still depends
on the supported SurfaceFlinger protocols: Android 10 (API 29) and Android 14
(API 34), with the Android 14 transaction layout checked against r35. Other
versions or incompatible builds are rejected. Android 14 capture and socket
streaming have been exercised on a Pixel 6a; Android 10 native-buffer capture
has not been verified on hardware.

A Linux arm64 build (`GOOS=linux`) uses the existing vendor-specific allocator
backends for the Android 14 r35 Pixel gchips layout, Android 10 Qualcomm SDM845
linear ION layout, and the tested Rockchip Bifrost gralloc3 ABI. Unknown layouts
are rejected.
