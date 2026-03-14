.PHONY: generate test e2e lint clean readme smoke aidlcli genaidlcli list-commands

# Generated top-level directories.
GENERATED_DIRS := android com fuzztest libgui_test_server parcelables src

# Generate all Go code from AOSP AIDL definitions.
generate:
	go run ./tools/cmd/aospgen -3rdparty tools/pkg/3rdparty -output . -smoke-tests

# Run unit tests (compiler + runtime packages).
test:
	go test ./tools/pkg/... ./binder/... ./parcel/... ./kernelbinder/... ./servicemanager/... ./errors/...

# Run E2E tests (requires /dev/binder or mock mode).
e2e:
	go test -tags e2e ./tests/e2e/... -count=1

# Run linter.
lint:
	golangci-lint run ./...

# Regenerate README package table.
readme:
	go run ./tools/cmd/genreadme

# Regenerate E2E smoke tests.
smoke:
	go run ./tools/cmd/gen_e2e_smoke .

# Regenerate aidlcli registry and command dispatch code.
genaidlcli:
	go run ./tools/cmd/genaidlcli

# Build the aidlcli tool.
aidlcli:
	go build -o aidlcli ./cmd/aidlcli

# List all available aidlcli subcommands.
list-commands:
	go run ./cmd/aidlcli --help 2>&1 | grep '^ ' | awk '{print $$1}'

# Remove all generated code.
clean:
	rm -rf $(GENERATED_DIRS)
	find . -maxdepth 1 -name '*.go' -exec grep -l 'Code generated' {} \; | xargs -r rm -f
