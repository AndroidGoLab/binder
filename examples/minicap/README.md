# Minicap example

This example captures a virtual display and serves minicap-compatible JPEG
frames on an Android abstract Unix socket. It also supports raw RGBA output.
Running it requires access to SurfaceFlinger and the graphics allocator; the
examples below use a privileged account.

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o minicap ./examples/minicap
adb push minicap /data/local/tmp/minicap
adb shell su 0 /data/local/tmp/minicap -P 1080x2400@360x800/0 -n minicap
```

Use the physical display dimensions for the first pair in `-P` and the maximum
output dimensions for the second. The last value is 0, 90, 180, or 270. The
output is fitted to the display's aspect ratio. Run `minicap -h` for the
options, including JPEG quality (`-Q` or `-q`), frame skipping (`-S`), frame
rate (`-r`), display information (`-i`), a capture test (`-t`), and a single
JPEG screenshot to stdout (`-s`). The default display is 0. Secondary-display
capture requires an available layer-stack mapping; Android 10 does not expose
that mapping through the supported native interface.

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

For unframed, tightly packed RGBA8888 frames on stdout, use `-raw`:

```sh
adb exec-out su 0 /data/local/tmp/minicap -P 1080x2400@360x800/0 -raw > frames.rgba
```

Each raw frame is `output_width * output_height * 4` bytes. For 90- or
270-degree projection, the output width and height are exchanged. Terminate
capture with SIGINT or SIGTERM.

The graphics backends recognize the Android 14 r35 Pixel gchips layout and
the Android 10 Qualcomm SDM845 uncached, linear ION layout. Unknown layouts
fail rather than being interpreted as pixels. Android 14 capture has been
exercised on a Pixel 6a; the Android 10 protocol and allocator have
source-based tests but have not been verified on Android 10 hardware. Other
Android versions and allocator layouts require additional support.
