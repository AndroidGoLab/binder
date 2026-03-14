package codegen

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
)

// GoFile builds a Go source file with automatic import management.
type GoFile struct {
	pkg     string
	imports map[string]string // import path -> alias (or "")
	buf     bytes.Buffer
}

// NewGoFile creates a new GoFile for the given package name.
func NewGoFile(pkg string) *GoFile {
	return &GoFile{
		pkg:     pkg,
		imports: make(map[string]string),
	}
}

// AddImport adds an import. alias can be empty for default import.
func (f *GoFile) AddImport(
	path string,
	alias string,
) {
	f.imports[path] = alias
}

// P writes a line (printf-style) to the file body.
func (f *GoFile) P(
	fmtStr string,
	args ...interface{},
) {
	fmt.Fprintf(&f.buf, fmtStr, args...)
	f.buf.WriteByte('\n')
}

// Bytes returns the formatted Go source code.
func (f *GoFile) Bytes() ([]byte, error) {
	var out bytes.Buffer

	fmt.Fprintf(&out, "package %s\n\n", f.pkg)

	if len(f.imports) > 0 {
		out.WriteString("import (\n")

		paths := make([]string, 0, len(f.imports))
		for p := range f.imports {
			paths = append(paths, p)
		}
		sort.Strings(paths)

		for _, p := range paths {
			alias := f.imports[p]
			if alias != "" {
				fmt.Fprintf(&out, "\t%s %q\n", alias, p)
			} else {
				fmt.Fprintf(&out, "\t%q\n", p)
			}
		}
		out.WriteString(")\n\n")
	}

	out.Write(f.buf.Bytes())

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return out.Bytes(), fmt.Errorf("gofmt failed: %w\nunformatted source:\n%s", err, out.Bytes())
	}
	return formatted, nil
}
