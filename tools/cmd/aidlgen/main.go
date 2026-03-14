package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/xaionaro-go/aidl/tools/pkg/codegen"
	"github.com/xaionaro-go/aidl/tools/pkg/resolver"
)

type searchPathsFlag []string

func (s *searchPathsFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *searchPathsFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	outputDir := flag.String("output", ".", "Output directory for generated Go files")
	var searchPaths searchPathsFlag
	flag.Var(&searchPaths, "I", "Search path for AIDL imports (can be repeated)")
	flag.Parse()

	files := flag.Args()

	// Backward compatibility: if no -I flags given and at least 2 positional args,
	// treat the first positional arg as the search path.
	if len(searchPaths) == 0 {
		if len(files) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: aidlgen [-output dir] -I <search-path> [-I <search-path>...] <aidl-files...>\n")
			fmt.Fprintf(os.Stderr, "       aidlgen [-output dir] <search-path> <aidl-files...>\n")
			os.Exit(1)
		}
		searchPaths = []string{files[0]}
		files = files[1:]
	}

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no AIDL files specified\n")
		os.Exit(1)
	}

	r := resolver.New(searchPaths)
	gen := codegen.NewGenerator(r, *outputDir)

	for _, f := range files {
		if err := r.ResolveFile(f); err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving %s: %v\n", f, err)
			os.Exit(1)
		}
	}

	if err := gen.GenerateAll(); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating code: %v\n", err)
		os.Exit(1)
	}
}
