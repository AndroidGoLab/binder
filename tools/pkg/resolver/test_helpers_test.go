package resolver

import (
	"github.com/AndroidGoLab/binder/tools/pkg/parser"
)

func parseTestFile(
	filename string,
) (*parser.Document, error) {
	return parser.ParseFile(filename)
}
