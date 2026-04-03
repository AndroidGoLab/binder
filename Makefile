.PHONY: specs generate cli readme test e2e e2e-bindercli vet build lint clean \
       bindercli list-commands check-generated release proofs difftest javaparser \
       gralloc-bridge

# Generated top-level directories.
GENERATED_DIRS := android com fuzztest libgui_test_server parcelables src

# All non-3rdparty Go packages.
GO_PACKAGES = $(shell go list -e ./... | grep -v /3rdparty/)

# --- Android NDK / GrapheneOS paths ---

NDK          := $(HOME)/Android/Sdk/ndk/28.0.13004108
NDK_CC       := $(NDK)/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android35-clang++
GRAPHENEOS   := /home/streaming/grapheneos
HIDL_GEN     := $(GRAPHENEOS)/out/soong/.intermediates

# --- Spec-first pipeline ---

# Baseline 3rdparty directory for param version diffing (API 35).
BASELINE_3RDPARTY := tools/pkg/3rdparty-api35
BASELINE_API := 35

# Baseline flags: pass only if the baseline submodule directory exists.
BASELINE_FLAGS := $(if $(wildcard $(BASELINE_3RDPARTY)/*),-baseline-3rdparty $(BASELINE_3RDPARTY) -baseline-api $(BASELINE_API))

# Extract specs from AIDL sources.
specs:
	go run ./tools/cmd/aidl2spec -3rdparty tools/pkg/3rdparty -output specs/ $(BASELINE_FLAGS)
	go run ./tools/cmd/java2spec -3rdparty tools/pkg/3rdparty -config tools/cmd/java2spec/constants.yaml -output specs/

# Extract specs with multi-version AOSP transaction code tables.
specs-versions:
	go run ./tools/cmd/aidl2spec -3rdparty tools/pkg/3rdparty -output specs/ -versions $(BASELINE_FLAGS)
	go run ./tools/cmd/java2spec -3rdparty tools/pkg/3rdparty -config tools/cmd/java2spec/constants.yaml -output specs/

# Generate all Go code from specs.
generate: specs
	go run ./tools/cmd/spec2go -specs specs/ -output . -native-impls native_impls/ -smoke-tests -codes-output binder/versionaware/codes_gen.go

# Generate bindercli commands from specs.
cli: specs
	go run ./tools/cmd/spec2cli -specs specs/ -output cmd/bindercli/

# Generate README from specs.
readme: specs
	go run ./tools/cmd/spec2readme -specs specs/ -output README.md

# --- Parser ---

# Regenerate ANTLR Java parser (requires Java 11+).
javaparser:
	cd tools/pkg/javaparser && ./generate.sh

# --- Proofs ---

# Build Lean 4 proofs (requires elan/lake toolchain).
proofs:
	cd proofs && lake build

# Run differential tests comparing Go against Lean oracle.
difftest: proofs
	go test -v ./tests/differential/

# --- Testing ---

# Run unit tests.
test:
	go test -v -race $(GO_PACKAGES)

# Run E2E tests on a connected device via adb.
e2e:
	@mkdir -p build
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -tags e2e -c -buildmode=pie -o build/e2e_test ./tests/e2e/
	patchelf --set-interpreter /system/bin/linker64 \
		--replace-needed libdl.so.2 libdl.so \
		--replace-needed libpthread.so.0 libc.so \
		--replace-needed libc.so.6 libc.so \
		build/e2e_test
	adb push build/e2e_test /data/local/tmp/
	adb shell /data/local/tmp/e2e_test -test.v -test.timeout 300s

# Run bindercli E2E tests via emulator.
e2e-bindercli:
	go test -tags e2e ./tests/e2e/... -run TestBindercli -v -timeout 300s

# Build gralloc bridge shared library for x86_64 Android using NDK.
# Requires stub libs (libhidlbase.so, libmapper3.so, libutils.so, libcutils.so)
# to be pulled from the emulator into /tmp first:
#   adb pull /system/lib64/libhidlbase.so /tmp/
#   adb pull /system/lib64/libutils.so /tmp/
#   adb pull /system/lib64/libcutils.so /tmp/
#   (libmapper3.so is a thin wrapper — pull or build separately)
gralloc-bridge:
	@mkdir -p build
	$(NDK_CC) -shared -fPIC -o build/gralloc_bridge.so gralloc/bridge/native/gralloc_bridge.cpp \
		-I$(GRAPHENEOS)/system/libhidl/transport/include \
		-I$(GRAPHENEOS)/system/libhidl/base/include \
		-I$(GRAPHENEOS)/system/core/libcutils/include \
		-I$(GRAPHENEOS)/system/core/libutils/include \
		-I$(GRAPHENEOS)/system/core/libsystem/include \
		-I$(GRAPHENEOS)/system/libfmq/base \
		-I$(HIDL_GEN)/hardware/interfaces/graphics/mapper/3.0/android.hardware.graphics.mapper@3.0_genc++_headers/gen \
		-I$(HIDL_GEN)/hardware/interfaces/graphics/common/1.0/android.hardware.graphics.common@1.0_genc++_headers/gen \
		-I$(HIDL_GEN)/hardware/interfaces/graphics/common/1.1/android.hardware.graphics.common@1.1_genc++_headers/gen \
		-I$(HIDL_GEN)/hardware/interfaces/graphics/common/1.2/android.hardware.graphics.common@1.2_genc++_headers/gen \
		-I$(HIDL_GEN)/hardware/interfaces/graphics/mapper/2.0/android.hardware.graphics.mapper@2.0_genc++_headers/gen \
		-I$(HIDL_GEN)/hardware/interfaces/graphics/mapper/2.1/android.hardware.graphics.mapper@2.1_genc++_headers/gen \
		-I$(HIDL_GEN)/system/libhidl/transport/base/1.0/android.hidl.base@1.0_genc++_headers/gen \
		-I$(HIDL_GEN)/system/libhidl/transport/manager/1.0/android.hidl.manager@1.0_genc++_headers/gen \
		-L/tmp -lhidlbase -lmapper3 -lutils -lcutils -std=c++17 -static-libstdc++

# --- Build ---

# Run go vet on all packages.
vet:
	go vet $(GO_PACKAGES)

# Build all commands, tools, and examples.
build:
	@mkdir -p build
	@for d in tools/cmd/*/; do echo "Building $$d..."; go build -o "build/$$(basename $$d)" "./$$d"; done
	@for d in cmd/*/; do echo "Building $$d..."; go build -o "build/$$(basename $$d)" "./$$d"; done
	@for d in examples/*/; do echo "Building $$d..."; go build -o "build/$$(basename $$d)" "./$$d"; done

# Build bindercli release binaries.
release:
	@mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o build/bindercli-linux-arm64 ./cmd/bindercli/
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/bindercli-linux-amd64 ./cmd/bindercli/

# Build the bindercli tool.
bindercli:
	@mkdir -p build
	go build -o build/bindercli ./cmd/bindercli

# Run linter.
lint:
	golangci-lint run ./...

# List all available bindercli subcommands.
list-commands:
	go run ./cmd/bindercli --help 2>&1 | grep '^ ' | awk '{print $$1}'

# Verify generated code matches a clean regeneration.
check-generated:
	make clean
	make generate
	make cli
	make readme
	git diff --exit-code

# Remove all generated code and specs (preserving hand-written overlay.yaml files).
clean:
	rm -rf $(GENERATED_DIRS)
	find specs -name spec.yaml -delete 2>/dev/null; find specs -type d -empty -delete 2>/dev/null; true
	find . -maxdepth 1 -name '*.go' -exec grep -l 'Code generated' {} \; | xargs -r rm -f
	rm -f servicemanager/service_names_gen.go
