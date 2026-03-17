.PHONY: generate genversions test e2e e2e-bindercli vet build build-examples lint clean readme smoke \
       bindercli genbindercli genservicemap genaccessors genparcelspec genparcelgo genconstants list-commands check-generated release

# Generated top-level directories.
GENERATED_DIRS := android com fuzztest libgui_test_server parcelables src

# All non-3rdparty Go packages.
GO_PACKAGES = $(shell go list -e ./... | grep -v /3rdparty/)

# Regenerate version-aware transaction code tables from AOSP tags.
genversions:
	go run ./tools/cmd/genaidl -3rdparty tools/pkg/3rdparty -output . -codes-output binder/versionaware/codes_gen.go -versions

# Generate all Go code from AOSP AIDL definitions.
generate:
	go run ./tools/cmd/genaidl -3rdparty tools/pkg/3rdparty -output . -smoke-tests

# Run unit tests (compiler + runtime packages).
test:
	go test -v -race $(GO_PACKAGES)

# Run E2E tests on a connected device via adb.
# Cross-compiles the test binary, pushes it, and runs on the device.
# bindercli tests are skipped (they require an emulator); on-device tests
# open /dev/binder directly.
e2e:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -tags e2e -c -o build/e2e_test ./tests/e2e/
	adb push build/e2e_test /data/local/tmp/
	adb shell /data/local/tmp/e2e_test -test.v -test.timeout 300s

# Run bindercli E2E tests via emulator (starts emulator if needed).
e2e-bindercli:
	go test -tags e2e ./tests/e2e/... -run TestBindercli -v -timeout 300s

# Run go vet on all packages.
vet:
	go vet $(GO_PACKAGES)

# Build all commands, tools, and examples.
build:
	@mkdir -p build
	@for d in tools/cmd/*/; do echo "Building $$d..."; go build -o "build/$$(basename $$d)" "./$$d"; done
	@for d in cmd/*/; do echo "Building $$d..."; go build -o "build/$$(basename $$d)" "./$$d"; done
	@for d in examples/*/; do echo "Building $$d..."; go build -o "build/$$(basename $$d)" "./$$d"; done

# Build bindercli release binaries for arm64 and amd64.
release:
	@mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o build/bindercli-linux-arm64 ./cmd/bindercli/
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o build/bindercli-linux-amd64 ./cmd/bindercli/

# Run linter.
lint:
	golangci-lint run ./...

# Regenerate README package table.
readme:
	go run ./tools/cmd/genreadme

# Regenerate E2E smoke tests.
smoke:
	go run ./tools/cmd/gen_e2e_smoke .

# Generate service name constants and JSON service map from AOSP Java sources.
genservicemap:
	go run ./tools/cmd/genservicemap \
		-frameworks-base tools/pkg/3rdparty/frameworks-base \
		-go-constants servicemanager/service_names_gen.go \
		-output /tmp/servicemap.json

# Generate typed service accessor functions from the service map.
genaccessors:
	go run ./tools/cmd/genaccessors \
		-service-map /tmp/servicemap.json \
		-output .

# Extract Java Parcelable wire formats into YAML specs.
genparcelspec:
	go run ./tools/cmd/genparcelspec \
		-frameworks-base tools/pkg/3rdparty/frameworks-base \
		-output parcelspecs/

# Generate Go marshal/unmarshal from Parcelable specs.
genparcelgo:
	go run ./tools/cmd/genparcelgo \
		-specs parcelspecs/ \
		-output .

# Generate typed Go constants from Java source files.
genconstants:
	go run ./tools/cmd/genconstants

# Regenerate bindercli registry and command dispatch code.
genbindercli:
	go run ./tools/cmd/genbindercli

# Build the bindercli tool.
bindercli:
	@mkdir -p build
	go build -o build/bindercli ./cmd/bindercli

# List all available bindercli subcommands.
list-commands:
	go run ./cmd/bindercli --help 2>&1 | grep '^ ' | awk '{print $$1}'

# Verify generated code matches a clean regeneration.
check-generated:
	make clean
	make generate
	make genparcelspec
	make genparcelgo
	make genservicemap
	make genaccessors
	make genconstants
	make smoke
	make readme
	git diff --exit-code

# Remove all generated code.
clean:
	rm -rf $(GENERATED_DIRS) parcelspecs
	find . -maxdepth 1 -name '*.go' -exec grep -l 'Code generated' {} \; | xargs -r rm -f
	rm -f servicemanager/service_names_gen.go
