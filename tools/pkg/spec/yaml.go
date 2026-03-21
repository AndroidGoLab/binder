package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ReadPackageSpec reads a single YAML spec file.
func ReadPackageSpec(
	path string,
) (*PackageSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var s PackageSpec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &s, nil
}

// WritePackageSpec writes a single YAML spec file.
// Creates parent directories as needed.
func WritePackageSpec(
	path string,
	s *PackageSpec,
) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshaling spec: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// ReadAllSpecs reads all spec.yaml files from the given root directory.
// Returns specs keyed by Go package path.
func ReadAllSpecs(
	rootDir string,
) (map[string]*PackageSpec, error) {
	specs := map[string]*PackageSpec{}

	err := filepath.Walk(rootDir, func(
		path string,
		info os.FileInfo,
		err error,
	) error {
		if err != nil {
			// Propagate walk errors instead of swallowing them.
			return err
		}
		if info.IsDir() || info.Name() != "spec.yaml" {
			return nil
		}

		s, readErr := ReadPackageSpec(path)
		if readErr != nil {
			return readErr
		}

		if _, exists := specs[s.GoPackage]; exists {
			return fmt.Errorf("duplicate GoPackage %q in %s", s.GoPackage, path)
		}
		specs[s.GoPackage] = s
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", rootDir, err)
	}

	return specs, nil
}

// WriteAllSpecs writes all specs to the given root directory.
// Each spec is written to rootDir/<go_package>/spec.yaml.
func WriteAllSpecs(
	rootDir string,
	specs map[string]*PackageSpec,
) error {
	// Write in sorted order for deterministic output.
	packages := make([]string, 0, len(specs))
	for pkg := range specs {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	for _, pkg := range packages {
		path := filepath.Join(rootDir, pkg, "spec.yaml")
		if err := WritePackageSpec(path, specs[pkg]); err != nil {
			return fmt.Errorf("writing %s: %w", pkg, err)
		}
	}

	return nil
}

// SpecPath returns the spec.yaml path for a Go package.
func SpecPath(
	rootDir string,
	goPackage string,
) string {
	return filepath.Join(rootDir, goPackage, "spec.yaml")
}

// GoPackageFromAIDL converts a dot-separated AIDL package name to a
// slash-separated Go package path, matching the existing codegen convention.
func GoPackageFromAIDL(aidlPackage string) string {
	return strings.ReplaceAll(aidlPackage, ".", "/")
}
