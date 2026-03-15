package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/xaionaro-go/binder/tools/pkg/servicemap"
)

func main() {
	frameworksBase := flag.String("frameworks-base", "tools/pkg/3rdparty/frameworks-base", "Path to the AOSP frameworks-base directory")
	output := flag.String("output", "", "Output file path for JSON (default: stdout)")
	flag.Parse()

	if err := run(*frameworksBase, *output); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(
	frameworksBase string,
	output string,
) error {
	svcMap, err := servicemap.BuildServiceMap(frameworksBase)
	if err != nil {
		return fmt.Errorf("building service map: %w", err)
	}

	// Build a sorted ordered map for deterministic JSON output.
	keys := make([]string, 0, len(svcMap))
	for k := range svcMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ordered := make(orderedMap, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, mapEntry{
			Key:   k,
			Value: svcMap[k],
		})
	}

	data, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	data = append(data, '\n')

	if output == "" {
		_, err = os.Stdout.Write(data)
		return err
	}

	return os.WriteFile(output, data, 0o644)
}

// mapEntry is one key-value pair in our ordered JSON object.
type mapEntry struct {
	Key   string
	Value servicemap.ServiceMapEntry
}

// orderedMap preserves insertion order when marshaled to JSON,
// producing an object with keys sorted by service name.
type orderedMap []mapEntry

func (o orderedMap) MarshalJSON() ([]byte, error) {
	// Build the JSON object manually to preserve key order.
	buf := []byte{'{'}
	for i, entry := range o {
		if i > 0 {
			buf = append(buf, ',')
		}

		key, err := json.Marshal(entry.Key)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(entry.Value)
		if err != nil {
			return nil, err
		}

		buf = append(buf, key...)
		buf = append(buf, ':')
		buf = append(buf, val...)
	}
	buf = append(buf, '}')
	return buf, nil
}
