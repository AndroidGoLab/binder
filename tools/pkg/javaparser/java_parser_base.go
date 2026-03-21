package javaparser

//go:generate ./generate.sh

import "github.com/antlr4-go/antlr/v4"

// JavaParserBase is the base struct for the generated JavaParser.
// It provides semantic predicate methods referenced by the grammar.
type JavaParserBase struct {
	*antlr.BaseParser
}

// DoLastRecordComponent checks whether the last record component in a
// RecordComponentList is valid (i.e., a varargs ELLIPSIS component must
// be the final one in the list).
func (b *JavaParserBase) DoLastRecordComponent() bool {
	ctx := b.GetParserRuleContext()
	rlCtx, ok := ctx.(*RecordComponentListContext)
	if !ok {
		return true
	}

	rcs := rlCtx.AllRecordComponent()
	if len(rcs) == 0 {
		return true
	}

	for i, rc := range rcs {
		rcCtx, ok := rc.(*RecordComponentContext)
		if !ok {
			continue
		}
		if rcCtx.ELLIPSIS() != nil && i+1 < len(rcs) {
			return false
		}
	}
	return true
}

// IsNotIdentifierAssign returns true unless the next two tokens are
// an identifier-like keyword followed by '='. This prevents the parser
// from treating "identifier = value" as an annotation field value.
func (b *JavaParserBase) IsNotIdentifierAssign() bool {
	la := b.GetTokenStream().LT(1).GetTokenType()

	switch la {
	case JavaParserIDENTIFIER,
		JavaParserMODULE,
		JavaParserOPEN,
		JavaParserREQUIRES,
		JavaParserEXPORTS,
		JavaParserOPENS,
		JavaParserTO,
		JavaParserUSES,
		JavaParserPROVIDES,
		JavaParserWHEN,
		JavaParserWITH,
		JavaParserTRANSITIVE,
		JavaParserYIELD,
		JavaParserSEALED,
		JavaParserPERMITS,
		JavaParserRECORD,
		JavaParserVAR:
		// Could be identifier = ..., check next token.
	default:
		return true
	}

	la2 := b.GetTokenStream().LT(2).GetTokenType()
	return la2 != JavaParserASSIGN
}
