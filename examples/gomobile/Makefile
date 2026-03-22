AAR = app/libs/binder.aar
MODULE = ../../interop/gomobile/client

.PHONY: aar apk install clean

aar: $(AAR)

$(AAR):
	mkdir -p app/libs
	gomobile bind -target android/arm64 -androidapi 21 -o $(AAR) $(MODULE)

apk: aar
	./gradlew assembleDebug

install: apk
	adb install -r app/build/outputs/apk/debug/app-debug.apk

clean:
	rm -f $(AAR)
	./gradlew clean
