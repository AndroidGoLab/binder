package dex

import (
	"os"
	"testing"
)

func BenchmarkExtractFromDEX(b *testing.B) {
	const path = "/tmp/classes4.dex"

	data, err := os.ReadFile(path)
	if err != nil {
		b.Skipf("skipping: %s not available: %v", path, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err := ExtractFromDEX(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExtractFromJAR(b *testing.B) {
	const path = "/tmp/framework.jar"

	if _, err := os.Stat(path); err != nil {
		b.Skipf("skipping: %s not available: %v", path, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err := ExtractFromJAR(path)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseDEXFile(b *testing.B) {
	const path = "/tmp/classes4.dex"

	data, err := os.ReadFile(path)
	if err != nil {
		b.Skipf("skipping: %s not available: %v", path, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err := parseDEXFile(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
