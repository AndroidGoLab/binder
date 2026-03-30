package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AndroidGoLab/binder/tools/pkg/codegen"
	"github.com/AndroidGoLab/binder/tools/pkg/resolver"
	"github.com/AndroidGoLab/binder/tools/pkg/spec"
)

// diffMethodParams compares oldMethod (from oldAPI) with newMethod (from newAPI)
// and sets MinAPILevel on params that were added in newAPI.
// Only trailing param additions are detected (AIDL stability guarantees
// params are never removed or reordered).
func diffMethodParams(
	oldMethod spec.MethodSpec,
	newMethod spec.MethodSpec,
	oldAPI int,
	newAPI int,
) []spec.ParamSpec {
	result := make([]spec.ParamSpec, len(newMethod.Params))
	copy(result, newMethod.Params)

	oldCount := len(oldMethod.Params)
	for i := range result {
		if i >= oldCount {
			result[i].MinAPILevel = newAPI
		}
	}
	return result
}

// diffBaselineParams parses AIDL files from the baseline 3rdparty directory,
// converts them to specs, and diffs each interface method's params against
// the current (new) specs. Any trailing params added in the new API get
// MinAPILevel = newAPI.
func diffBaselineParams(
	baseline3rdparty string,
	baselineAPI int,
	newAPI int,
	currentSpecs map[string]*spec.PackageSpec,
) error {
	absBaseline, err := filepath.Abs(baseline3rdparty)
	if err != nil {
		return fmt.Errorf("resolving baseline path: %w", err)
	}

	if _, err := os.Stat(absBaseline); os.IsNotExist(err) {
		return fmt.Errorf("baseline 3rdparty directory not found: %s", absBaseline)
	}

	fmt.Fprintf(os.Stderr, "Diffing params against baseline API %d from %s...\n", baselineAPI, absBaseline)

	aidlFiles, err := discoverAIDLFiles(absBaseline)
	if err != nil {
		return fmt.Errorf("discovering baseline AIDL files: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Baseline: found %d AIDL files\n", len(aidlFiles))

	searchRoots, err := discoverSearchRoots(aidlFiles)
	if err != nil {
		return fmt.Errorf("discovering baseline search roots: %w", err)
	}

	r := resolver.New(searchRoots)
	r.SetSkipUnresolved(true)

	var parseFailCount int
	for _, f := range aidlFiles {
		if err := r.ResolveFile(f); err != nil {
			parseFailCount++
			continue
		}
	}

	allDefs := r.Registry.All()
	baselineSpecs := convertToSpecs(allDefs)
	fmt.Fprintf(os.Stderr, "Baseline: %d packages (%d parse failures)\n", len(baselineSpecs), parseFailCount)

	// Build an index of baseline interface methods by descriptor+method name.
	type methodKey struct {
		goPkg      string
		ifaceName  string
		methodName string
	}
	baselineMethods := map[methodKey]spec.MethodSpec{}
	for goPkg, ps := range baselineSpecs {
		for _, iface := range ps.Interfaces {
			for _, m := range iface.Methods {
				baselineMethods[methodKey{goPkg, iface.Name, m.Name}] = m
			}
		}
	}

	// Diff each method in current specs against baseline.
	var diffCount int
	for goPkg, ps := range currentSpecs {
		for i := range ps.Interfaces {
			iface := &ps.Interfaces[i]
			for j := range iface.Methods {
				m := &iface.Methods[j]
				key := methodKey{
					goPkg:      codegen.AIDLToGoPackage(ps.AIDLPackage),
					ifaceName:  iface.Name,
					methodName: m.Name,
				}
				baselineMethod, ok := baselineMethods[key]
				if !ok {
					// Method not in baseline — all params are new.
					// But we only annotate if the baseline package
					// existed (i.e., the interface was present).
					_, baselinePkgExists := baselineSpecs[goPkg]
					if !baselinePkgExists {
						continue
					}
					// Interface exists in baseline but method doesn't:
					// the method was added in the new API. We don't
					// annotate individual params since the whole method
					// is new (handled by ResolveCode returning an error).
					continue
				}
				if len(m.Params) > len(baselineMethod.Params) {
					m.Params = diffMethodParams(baselineMethod, *m, baselineAPI, newAPI)
					diffCount++
				}
			}
		}
	}

	fmt.Fprintf(os.Stderr, "Annotated %d methods with version-dependent params\n", diffCount)
	return nil
}
