package codegen

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
)

// GoFile builds a Go source file with automatic import management.
type GoFile struct {
	Pkg     string
	Imports map[string]string // import path -> alias (or "")
	Buf     bytes.Buffer
}

// NewGoFile creates a new GoFile for the given package name.
func NewGoFile(pkg string) *GoFile {
	return &GoFile{
		Pkg:     pkg,
		Imports: make(map[string]string),
	}
}

// AddImport adds an import. alias can be empty for default import.
func (f *GoFile) AddImport(
	path string,
	alias string,
) {
	f.Imports[path] = alias
}

// P writes a line (printf-style) to the file body.
func (f *GoFile) P(
	fmtStr string,
	args ...any,
) {
	fmt.Fprintf(&f.Buf, fmtStr, args...)
	f.Buf.WriteByte('\n')
}

// Bytes returns the formatted Go source code.
func (f *GoFile) Bytes() ([]byte, error) {
	var out bytes.Buffer

	fmt.Fprintf(&out, "package %s\n\n", f.Pkg)

	if len(f.Imports) > 0 {
		out.WriteString("import (\n")

		paths := make([]string, 0, len(f.Imports))
		for p := range f.Imports {
			paths = append(paths, p)
		}
		sort.Strings(paths)

		for _, p := range paths {
			alias := f.Imports[p]
			if alias != "" {
				fmt.Fprintf(&out, "\t%s %q\n", alias, p)
			} else {
				fmt.Fprintf(&out, "\t%q\n", p)
			}
		}
		out.WriteString(")\n\n")
	}

	out.Write(f.Buf.Bytes())

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return out.Bytes(), fmt.Errorf("gofmt failed: %w\nunformatted source:\n%s", err, out.Bytes())
	}
	return formatted, nil
}
