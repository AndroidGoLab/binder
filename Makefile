.PHONY: specs generate cli readme test e2e e2e-bindercli vet build lint clean \
       bindercli list-commands check-generated release proofs difftest javaparser

# Generated top-level directories.
GENERATED_DIRS := android com fuzztest libgui_test_server parcelables src

# All non-3rdparty Go packages.
GO_PACKAGES = $(shell go list -e ./... | grep -v /3rdparty/)

# --- Spec-first pipeline ---

# Extract specs from AIDL sources.
specs:
	go run ./tools/cmd/aidl2spec -3rdparty tools/pkg/3rdparty -output specs/
	go run ./tools/cmd/java2spec -3rdparty tools/pkg/3rdparty -config tools/cmd/java2spec/constants.yaml -output specs/

# Extract specs with multi-version AOSP transaction code tables.
specs-versions:
	go run ./tools/cmd/aidl2spec -3rdparty tools/pkg/3rdparty -output specs/ -versions
	go run ./tools/cmd/java2spec -3rdparty tools/pkg/3rdparty -config tools/cmd/java2spec/constants.yaml -output specs/

# Generate all Go code from specs.
generate: specs
	go run ./tools/cmd/spec2go -specs specs/ -output . -smoke-tests -codes-output binder/versionaware/codes_gen.go

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
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -tags e2e -c -o build/e2e_test ./tests/e2e/
	adb push build/e2e_test /data/local/tmp/
	adb shell /data/local/tmp/e2e_test -test.v -test.timeout 300s

# Run bindercli E2E tests via emulator.
e2e-bindercli:
	go test -tags e2e ./tests/e2e/... -run TestBindercli -v -timeout 300s

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

# Remove all generated code and specs.
clean:
	rm -rf $(GENERATED_DIRS) specs
	find . -maxdepth 1 -name '*.go' -exec grep -l 'Code generated' {} \; | xargs -r rm -f
	rm -f servicemanager/service_names_gen.go
