package parser

import (
	"fmt"
	"os"
)

// Parse parses AIDL source code and returns the AST document.
func Parse(
	filename string,
	src []byte,
) (*Document, error) {
	p := &parserState{
		lex: NewLexer(filename, src),
	}
	p.advance()
	return p.parseDocument()
}

// ParseFile reads a file and parses it as AIDL.
func ParseFile(
	filename string,
) (*Document, error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}
	return Parse(filename, src)
}

// parserState holds the state of the recursive-descent parser.
type parserState struct {
	lex      *Lexer
	cur      Token
	lexErr   error
	pushback *Token // synthetic token to return before the next lex call
}

func (p *parserState) advance() Token {
	prev := p.cur
	if p.pushback != nil {
		p.cur = *p.pushback
		p.pushback = nil
	} else {
		p.cur = p.lex.Next()
	}
	if p.cur.Kind == TokenError {
		p.lexErr = fmt.Errorf("%s: %s", p.cur.Pos, p.cur.Value)
	}
	return prev
}

func (p *parserState) at(
	kind TokenKind,
) bool {
	return p.cur.Kind == kind
}

func (p *parserState) expect(
	kind TokenKind,
) (Token, error) {
	if p.lexErr != nil {
		return Token{}, p.lexErr
	}
	if p.cur.Kind != kind {
		return Token{}, fmt.Errorf(
			"%s: expected %s, got %s (%q)",
			p.cur.Pos, kind, p.cur.Kind, p.cur.Value,
		)
	}
	return p.advance(), nil
}

// expectRAngle consumes a '>' token. When the current token is '>>' (RShift),
// it splits it into two '>' tokens, consuming one and pushing back the other.
// This handles nested generics like Map<String, List<Foo>>.
func (p *parserState) expectRAngle() error {
	if p.lexErr != nil {
		return p.lexErr
	}

	if p.cur.Kind == TokenRAngle {
		p.advance()
		return nil
	}

	if p.cur.Kind == TokenRShift {
		// Split ">>" into ">" consumed now, and ">" pushed back.
		synth := Token{
			Kind:  TokenRAngle,
			Pos:   p.cur.Pos,
			Value: ">",
		}
		synth.Pos.Column++
		p.pushback = &synth
		p.advance()
		return nil
	}

	return fmt.Errorf(
		"%s: expected >, got %s (%q)",
		p.cur.Pos, p.cur.Kind, p.cur.Value,
	)
}

func (p *parserState) parseDocument() (*Document, error) {
	if p.lexErr != nil {
		return nil, p.lexErr
	}

	doc := &Document{}

	// Optional package declaration.
	if p.at(TokenPackage) {
		pkg, err := p.parsePackage()
		if err != nil {
			return nil, err
		}
		doc.Package = pkg
	}

	// Import declarations.
	for p.at(TokenImport) {
		imp, err := p.parseImport()
		if err != nil {
			return nil, err
		}
		doc.Imports = append(doc.Imports, imp)
	}

	// Top-level definitions.
	for !p.at(TokenEOF) {
		if p.lexErr != nil {
			return nil, p.lexErr
		}
		def, err := p.parseDefinition()
		if err != nil {
			return nil, err
		}
		doc.Definitions = append(doc.Definitions, def)
	}

	return doc, nil
}

func (p *parserState) parsePackage() (*PackageDecl, error) {
	tok, err := p.expect(TokenPackage)
	if err != nil {
		return nil, err
	}

	name, err := p.parseDottedName()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenSemicolon); err != nil {
		return nil, err
	}

	return &PackageDecl{Pos: tok.Pos, Name: name}, nil
}

func (p *parserState) parseImport() (*ImportDecl, error) {
	tok, err := p.expect(TokenImport)
	if err != nil {
		return nil, err
	}

	name, err := p.parseDottedName()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenSemicolon); err != nil {
		return nil, err
	}

	return &ImportDecl{Pos: tok.Pos, Name: name}, nil
}

func (p *parserState) parseDottedName() (string, error) {
	tok, err := p.expect(TokenIdent)
	if err != nil {
		return "", err
	}

	name := tok.Value
	for p.at(TokenDot) {
		p.advance()
		part, err := p.expect(TokenIdent)
		if err != nil {
			return "", err
		}
		name += "." + part.Value
	}

	return name, nil
}

func (p *parserState) parseDefinition() (Definition, error) {
	annots, err := p.parseAnnotations()
	if err != nil {
		return nil, err
	}

	switch p.cur.Kind {
	case TokenOneway:
		return p.parseInterface(annots, true)
	case TokenInterface:
		return p.parseInterface(annots, false)
	case TokenParcelable:
		return p.parseParcelable(annots)
	case TokenEnum:
		return p.parseEnum(annots)
	case TokenUnion:
		return p.parseUnion(annots)
	default:
		return nil, fmt.Errorf(
			"%s: expected definition (interface, parcelable, enum, union), got %s (%q)",
			p.cur.Pos, p.cur.Kind, p.cur.Value,
		)
	}
}

func (p *parserState) parseInterface(
	annots []*Annotation,
	oneway bool,
) (*InterfaceDecl, error) {
	if oneway {
		p.advance() // consume "oneway"
	}

	tok, err := p.expect(TokenInterface)
	if err != nil {
		return nil, err
	}

	nameTok, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}

	decl := &InterfaceDecl{
		Pos:      tok.Pos,
		Annots:   annots,
		IntfName: nameTok.Value,
		Oneway:   oneway,
	}

	for !p.at(TokenRBrace) && !p.at(TokenEOF) {
		switch {
		case p.at(TokenConst):
			c, err := p.parseConstant()
			if err != nil {
				return nil, err
			}
			decl.Constants = append(decl.Constants, c)
		case p.isNestedTypeStart():
			nested, err := p.parseInterfaceMemberDefinition()
			if err != nil {
				return nil, err
			}
			decl.NestedTypes = append(decl.NestedTypes, nested)
		default:
			m, err := p.parseMethod()
			if err != nil {
				return nil, err
			}
			decl.Methods = append(decl.Methods, m)
		}
	}

	if _, err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}

	return decl, nil
}

// isNestedTypeStart checks whether the current position starts a nested type
// definition. It handles annotations before the type keyword, and the
// "oneway interface" pattern.
func (p *parserState) isNestedTypeStart() bool {
	// Direct type keyword without annotations.
	if isNestedTypeKeyword(p.cur.Kind) {
		return true
	}

	// "oneway interface" pattern for nested oneway interfaces.
	if p.at(TokenOneway) && p.peekNextTokenKind() == TokenInterface {
		return true
	}

	// Annotations followed by a type keyword are also nested type definitions.
	// We peek ahead past annotations to see if a type keyword follows.
	if !p.at(TokenAnnotation) {
		return false
	}

	return p.peekPastAnnotationsIsTypeKeyword()
}

// peekPastAnnotationsIsTypeKeyword looks ahead in the lexer source (without
// consuming tokens) to check if a sequence of annotations is followed by a
// type keyword (enum, parcelable, union, interface).
//
// Precondition: p.cur.Kind == TokenAnnotation.
func (p *parserState) peekPastAnnotationsIsTypeKeyword() bool {
	// Save lexer state to restore later.
	savedOffset := p.lex.Offset
	savedLine := p.lex.Line
	savedColumn := p.lex.Column
	defer func() {
		p.lex.Offset = savedOffset
		p.lex.Line = savedLine
		p.lex.Column = savedColumn
	}()

	// p.cur is already an annotation token. The lexer is positioned just
	// after the annotation name. We need to skip any params of the current
	// annotation, then check for more annotations or a type keyword.
	for {
		// Skip annotation params if present (the '(' ... ')' block).
		if err := p.lex.skipWhitespaceAndComments(); err != nil {
			return false
		}
		if p.lex.Offset < len(p.lex.Src) && p.lex.Src[p.lex.Offset] == '(' {
			depth := 1
			p.lex.advance()
			for depth > 0 && p.lex.Offset < len(p.lex.Src) {
				ch := p.lex.advance()
				switch ch {
				case '(':
					depth++
				case ')':
					depth--
				}
			}
		}

		// Get the next token after the annotation (and its params).
		tok := p.lex.Next()
		if tok.Kind == TokenAnnotation {
			// Another annotation follows; continue the loop to skip its params.
			continue
		}

		// "oneway interface" pattern: oneway followed by interface keyword.
		if tok.Kind == TokenOneway {
			next := p.lex.Next()
			return next.Kind == TokenInterface
		}

		return isNestedTypeKeyword(tok.Kind)
	}
}

func (p *parserState) parseMethod() (*MethodDecl, error) {
	annots, err := p.parseAnnotations()
	if err != nil {
		return nil, err
	}

	pos := p.cur.Pos
	oneway := false
	if p.at(TokenOneway) {
		oneway = true
		p.advance()
	}

	retType, err := p.parseTypeSpecifier()
	if err != nil {
		return nil, err
	}

	nameTok, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	var params []*ParamDecl
	for !p.at(TokenRParen) && !p.at(TokenEOF) {
		if len(params) > 0 {
			if _, err := p.expect(TokenComma); err != nil {
				return nil, err
			}
		}

		param, err := p.parseParam()
		if err != nil {
			return nil, err
		}
		params = append(params, param)
	}

	if _, err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	transactionID := 0
	if p.at(TokenAssign) {
		p.advance()
		idTok, err := p.expect(TokenIntLiteral)
		if err != nil {
			return nil, err
		}

		val, err := parseIntString(idTok.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid transaction ID: %w", idTok.Pos, err)
		}
		transactionID = int(val)
	}

	if _, err := p.expect(TokenSemicolon); err != nil {
		return nil, err
	}

	return &MethodDecl{
		Pos:           pos,
		Annots:        annots,
		Oneway:        oneway,
		ReturnType:    retType,
		MethodName:    nameTok.Value,
		Params:        params,
		TransactionID: transactionID,
	}, nil
}

func (p *parserState) parseParam() (*ParamDecl, error) {
	annots, err := p.parseAnnotations()
	if err != nil {
		return nil, err
	}

	pos := p.cur.Pos
	dir := DirectionNone
	switch p.cur.Kind {
	case TokenIn:
		dir = DirectionIn
		p.advance()
	case TokenOut:
		dir = DirectionOut
		p.advance()
	case TokenInout:
		dir = DirectionInOut
		p.advance()
	}

	typ, err := p.parseTypeSpecifier()
	if err != nil {
		return nil, err
	}

	nameTok, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}

	return &ParamDecl{
		Pos:       pos,
		Annots:    annots,
		Direction: dir,
		Type:      typ,
		ParamName: nameTok.Value,
	}, nil
}

func (p *parserState) parseParcelable(
	annots []*Annotation,
) (*ParcelableDecl, error) {
	tok, err := p.expect(TokenParcelable)
	if err != nil {
		return nil, err
	}

	nameTok, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}

	// Support dotted names in forward declarations (e.g., parcelable Foo.Bar;).
	name := nameTok.Value
	for p.at(TokenDot) {
		p.advance()
		part, err := p.expect(TokenIdent)
		if err != nil {
			return nil, err
		}
		name += "." + part.Value
	}

	decl := &ParcelableDecl{
		Pos:      tok.Pos,
		Annots:   annots,
		ParcName: name,
	}

	// Generic type parameters on declarations (e.g., parcelable Foo<T>).
	if p.at(TokenLAngle) {
		if err := p.skipTypeParams(); err != nil {
			return nil, err
		}
	}

	// Forward-declared parcelable with foreign language headers.
	if p.at(TokenIdent) && isForeignHeaderDirective(p.cur.Value) {
		if err := p.parseForeignHeaders(decl); err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenSemicolon); err != nil {
			return nil, err
		}
		return decl, nil
	}

	// Forward-declared parcelable without body (just semicolon).
	if p.at(TokenSemicolon) {
		p.advance()
		return decl, nil
	}

	if _, err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}

	for !p.at(TokenRBrace) && !p.at(TokenEOF) {
		switch {
		case p.at(TokenConst):
			c, err := p.parseConstant()
			if err != nil {
				return nil, err
			}
			decl.Constants = append(decl.Constants, c)
		case p.isNestedTypeStart():
			nested, err := p.parseInterfaceMemberDefinition()
			if err != nil {
				return nil, err
			}
			decl.NestedTypes = append(decl.NestedTypes, nested)
		default:
			f, err := p.parseField()
			if err != nil {
				return nil, err
			}
			decl.Fields = append(decl.Fields, f)
		}
	}

	if _, err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}

	return decl, nil
}

func (p *parserState) parseField() (*FieldDecl, error) {
	annots, err := p.parseAnnotations()
	if err != nil {
		return nil, err
	}

	pos := p.cur.Pos

	typ, err := p.parseTypeSpecifier()
	if err != nil {
		return nil, err
	}

	nameTok, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}

	var defaultVal ConstExpr
	if p.at(TokenAssign) {
		p.advance()
		defaultVal, err = p.parseConstExpr()
		if err != nil {
			return nil, err
		}
	}

	if _, err := p.expect(TokenSemicolon); err != nil {
		return nil, err
	}

	return &FieldDecl{
		Pos:          pos,
		Annots:       annots,
		Type:         typ,
		FieldName:    nameTok.Value,
		DefaultValue: defaultVal,
	}, nil
}

func (p *parserState) parseEnum(
	annots []*Annotation,
) (*EnumDecl, error) {
	tok, err := p.expect(TokenEnum)
	if err != nil {
		return nil, err
	}

	nameTok, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}

	decl := &EnumDecl{
		Pos:      tok.Pos,
		Annots:   annots,
		EnumName: nameTok.Value,
	}

	// Extract backing type from @Backing annotation.
	for _, a := range annots {
		if a.Name == "Backing" {
			if typeExpr, ok := a.Params["type"]; ok {
				if strLit, ok := typeExpr.(*StringLiteralExpr); ok {
					decl.BackingType = &TypeSpecifier{
						Pos:  a.Pos,
						Name: strLit.Value,
					}
				}
			}
		}
	}

	for !p.at(TokenRBrace) && !p.at(TokenEOF) {
		e, err := p.parseEnumerator()
		if err != nil {
			return nil, err
		}
		decl.Enumerators = append(decl.Enumerators, e)

		// Trailing comma is optional; consume if present.
		if p.at(TokenComma) {
			p.advance()
		}
	}

	if _, err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}

	return decl, nil
}

func (p *parserState) parseEnumerator() (*Enumerator, error) {
	nameTok, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}

	e := &Enumerator{
		Pos:  nameTok.Pos,
		Name: nameTok.Value,
	}

	if p.at(TokenAssign) {
		p.advance()
		val, err := p.parseConstExpr()
		if err != nil {
			return nil, err
		}
		e.Value = val
	}

	return e, nil
}

func (p *parserState) parseUnion(
	annots []*Annotation,
) (*UnionDecl, error) {
	tok, err := p.expect(TokenUnion)
	if err != nil {
		return nil, err
	}

	nameTok, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}

	decl := &UnionDecl{
		Pos:       tok.Pos,
		Annots:    annots,
		UnionName: nameTok.Value,
	}

	for !p.at(TokenRBrace) && !p.at(TokenEOF) {
		switch {
		case p.at(TokenConst):
			c, err := p.parseConstant()
			if err != nil {
				return nil, err
			}
			decl.Constants = append(decl.Constants, c)
		case p.isNestedTypeStart():
			nested, err := p.parseInterfaceMemberDefinition()
			if err != nil {
				return nil, err
			}
			decl.NestedTypes = append(decl.NestedTypes, nested)
		default:
			f, err := p.parseField()
			if err != nil {
				return nil, err
			}
			decl.Fields = append(decl.Fields, f)
		}
	}

	if _, err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}

	return decl, nil
}

func (p *parserState) parseConstant() (*ConstantDecl, error) {
	tok, err := p.expect(TokenConst)
	if err != nil {
		return nil, err
	}

	typ, err := p.parseTypeSpecifier()
	if err != nil {
		return nil, err
	}

	nameTok, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenAssign); err != nil {
		return nil, err
	}

	val, err := p.parseConstExpr()
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenSemicolon); err != nil {
		return nil, err
	}

	return &ConstantDecl{
		Pos:       tok.Pos,
		Type:      typ,
		ConstName: nameTok.Value,
		Value:     val,
	}, nil
}

func (p *parserState) parseTypeSpecifier() (*TypeSpecifier, error) {
	annots, err := p.parseAnnotations()
	if err != nil {
		return nil, err
	}

	pos := p.cur.Pos

	// The type name. "void" is also valid as a return type.
	var name string
	if p.at(TokenVoid) {
		name = "void"
		p.advance()
	} else {
		name, err = p.parseDottedName()
		if err != nil {
			return nil, err
		}
	}

	ts := &TypeSpecifier{
		Pos:    pos,
		Annots: annots,
		Name:   name,
	}

	// Generic type arguments: <T, U>
	if p.at(TokenLAngle) {
		p.advance()
		for {
			arg, err := p.parseTypeSpecifier()
			if err != nil {
				return nil, err
			}
			ts.TypeArgs = append(ts.TypeArgs, arg)

			if !p.at(TokenComma) {
				break
			}
			p.advance()
		}

		if err := p.expectRAngle(); err != nil {
			return nil, err
		}
	}

	// Array suffix: [], [N], or [CONST_NAME]
	if p.at(TokenLBracket) {
		p.advance()
		ts.IsArray = true

		switch {
		case p.at(TokenIntLiteral):
			tok := p.advance()
			ts.FixedSize = tok.Value
		case p.at(TokenIdent):
			tok := p.advance()
			ts.FixedSize = tok.Value
		}

		if _, err := p.expect(TokenRBracket); err != nil {
			return nil, err
		}
	}

	return ts, nil
}

func (p *parserState) parseAnnotations() ([]*Annotation, error) {
	var annots []*Annotation

	for p.at(TokenAnnotation) {
		a, err := p.parseAnnotation()
		if err != nil {
			return nil, err
		}
		annots = append(annots, a)
	}

	return annots, nil
}

func (p *parserState) parseAnnotation() (*Annotation, error) {
	tok := p.advance() // TokenAnnotation

	a := &Annotation{
		Pos:  tok.Pos,
		Name: tok.Value,
	}

	// Optional parameters in parentheses.
	if p.at(TokenLParen) {
		p.advance()
		a.Params = make(map[string]ConstExpr)

		paramIdx := 0
		for !p.at(TokenRParen) && !p.at(TokenEOF) {
			if paramIdx > 0 {
				if _, err := p.expect(TokenComma); err != nil {
					return nil, err
				}
			}

			// Determine if this is key=value or a positional value.
			// If we see an identifier followed by '=', it's key=value.
			// We must distinguish '=' (assign) from '==' (equality).
			// peekNextNonWS returns '=' for both, so after consuming
			// the ident we verify the token is TokenAssign, not TokenEqEq.
			if p.at(TokenIdent) && p.lex.peekNextNonWS() == '=' {
				keyTok := p.advance()
				if p.at(TokenAssign) {
					p.advance() // consume '='
					val, err := p.parseConstExpr()
					if err != nil {
						return nil, err
					}
					a.Params[keyTok.Value] = val
				} else {
					// It was '==' not '=': push the ident back as
					// part of a value expression. We already consumed
					// the ident, so wrap it and parse the rest via
					// the equality operator in parseConstExpr.
					p.pushback = &p.cur
					p.cur = keyTok
					val, err := p.parseAnnotationValue()
					if err != nil {
						return nil, err
					}
					a.Params["value"] = val
				}
			} else {
				// Positional value: store under "value" key.
				val, err := p.parseAnnotationValue()
				if err != nil {
					return nil, err
				}
				a.Params["value"] = val
			}
			paramIdx++
		}

		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
	}

	return a, nil
}

// parseAnnotationValue parses an annotation value, which can be a constant
// expression or a brace-enclosed list of values like {"a", "b"}.
func (p *parserState) parseAnnotationValue() (ConstExpr, error) {
	return p.parseConstExpr()
}

// parseInterfaceMemberDefinition parses a nested type definition inside an
// interface, parcelable, or union body. It handles leading annotations and
// the "oneway interface" pattern.
func (p *parserState) parseInterfaceMemberDefinition() (Definition, error) {
	annots, err := p.parseAnnotations()
	if err != nil {
		return nil, err
	}

	// Handle "oneway interface" pattern.
	if p.at(TokenOneway) {
		return p.parseInterface(annots, true)
	}

	return p.parseNestedDefinition(annots)
}

// peekNextTokenKind peeks at the next token kind without consuming it. It saves
// and restores the lexer state.
func (p *parserState) peekNextTokenKind() TokenKind {
	savedOffset := p.lex.Offset
	savedLine := p.lex.Line
	savedColumn := p.lex.Column
	defer func() {
		p.lex.Offset = savedOffset
		p.lex.Line = savedLine
		p.lex.Column = savedColumn
	}()

	tok := p.lex.Next()
	return tok.Kind
}

// isNestedTypeKeyword returns true if the given token kind is a type keyword
// that can start a nested type definition.
func isNestedTypeKeyword(kind TokenKind) bool {
	switch kind {
	case TokenEnum, TokenParcelable, TokenUnion, TokenInterface:
		return true
	}
	return false
}

// parseNestedDefinition parses a nested type definition (enum, parcelable,
// union, or interface) inside another type body.
func (p *parserState) parseNestedDefinition(
	annots []*Annotation,
) (Definition, error) {
	switch p.cur.Kind {
	case TokenEnum:
		return p.parseEnum(annots)
	case TokenParcelable:
		return p.parseParcelable(annots)
	case TokenUnion:
		return p.parseUnion(annots)
	case TokenInterface:
		return p.parseInterface(annots, false)
	default:
		return nil, fmt.Errorf(
			"%s: expected nested type definition, got %s (%q)",
			p.cur.Pos, p.cur.Kind, p.cur.Value,
		)
	}
}

// isForeignHeaderDirective returns true if the given identifier is a foreign
// language header directive used in forward-declared parcelables.
func isForeignHeaderDirective(name string) bool {
	switch name {
	case "cpp_header", "ndk_header", "rust_type":
		return true
	}
	return false
}

// parseForeignHeaders parses one or more foreign language header directives
// (cpp_header, ndk_header, rust_type) on a forward-declared parcelable.
func (p *parserState) parseForeignHeaders(
	decl *ParcelableDecl,
) error {
	for p.at(TokenIdent) && isForeignHeaderDirective(p.cur.Value) {
		directive := p.cur.Value
		p.advance()

		str, err := p.expect(TokenStringLiteral)
		if err != nil {
			return err
		}

		switch directive {
		case "cpp_header":
			decl.CppHeader = str.Value
		case "ndk_header":
			decl.NdkHeader = str.Value
		case "rust_type":
			decl.RustType = str.Value
		}
	}
	return nil
}

// skipTypeParams skips a generic type parameter list (<T>, <T, U>) on a
// declaration. This handles declarations like parcelable Foo<T>.
func (p *parserState) skipTypeParams() error {
	if _, err := p.expect(TokenLAngle); err != nil {
		return err
	}

	depth := 1
	for depth > 0 && !p.at(TokenEOF) {
		switch p.cur.Kind {
		case TokenLAngle:
			depth++
		case TokenRAngle:
			depth--
		case TokenRShift:
			// ">>" counts as two closing angle brackets.
			// Clamp to zero to avoid underflow when ">>" closes the
			// last level (depth was 1 before this token).
			depth -= 2
			if depth < 0 {
				depth = 0
			}
		}
		p.advance()
	}
	return nil
}

// parseConstExpr parses a constant expression using Pratt parsing.
func (p *parserState) parseConstExpr() (ConstExpr, error) {
	return p.parseTernary()
}

func (p *parserState) parseTernary() (ConstExpr, error) {
	expr, err := p.parseLogicalOr()
	if err != nil {
		return nil, err
	}

	if p.at(TokenQuestion) {
		pos := p.cur.Pos
		p.advance()

		then, err := p.parseConstExpr()
		if err != nil {
			return nil, err
		}

		if _, err := p.expect(TokenColon); err != nil {
			return nil, err
		}

		elseExpr, err := p.parseTernary()
		if err != nil {
			return nil, err
		}

		expr = &TernaryExpr{
			TokenPos: pos,
			Cond:     expr,
			Then:     then,
			Else:     elseExpr,
		}
	}

	return expr, nil
}

func (p *parserState) parseLogicalOr() (ConstExpr, error) {
	left, err := p.parseLogicalAnd()
	if err != nil {
		return nil, err
	}

	for p.at(TokenPipePipe) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseLogicalAnd()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseLogicalAnd() (ConstExpr, error) {
	left, err := p.parseBitwiseOr()
	if err != nil {
		return nil, err
	}

	for p.at(TokenAmpAmp) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseBitwiseOr()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseBitwiseOr() (ConstExpr, error) {
	left, err := p.parseBitwiseXor()
	if err != nil {
		return nil, err
	}

	for p.at(TokenPipe) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseBitwiseXor()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseBitwiseXor() (ConstExpr, error) {
	left, err := p.parseBitwiseAnd()
	if err != nil {
		return nil, err
	}

	for p.at(TokenCaret) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseBitwiseAnd()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseBitwiseAnd() (ConstExpr, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}

	for p.at(TokenAmp) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseEquality() (ConstExpr, error) {
	left, err := p.parseRelational()
	if err != nil {
		return nil, err
	}

	for p.at(TokenEqEq) || p.at(TokenBangEq) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseRelational()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseRelational() (ConstExpr, error) {
	left, err := p.parseShift()
	if err != nil {
		return nil, err
	}

	for p.at(TokenLAngle) || p.at(TokenRAngle) || p.at(TokenLessEq) || p.at(TokenGreaterEq) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseShift()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseShift() (ConstExpr, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}

	for p.at(TokenLShift) || p.at(TokenRShift) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseAdditive() (ConstExpr, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}

	for p.at(TokenPlus) || p.at(TokenMinus) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseMultiplicative() (ConstExpr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	for p.at(TokenStar) || p.at(TokenSlash) || p.at(TokenPercent) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{TokenPos: pos, Op: op, Left: left, Right: right}
	}

	return left, nil
}

func (p *parserState) parseUnary() (ConstExpr, error) {
	if p.at(TokenMinus) || p.at(TokenTilde) || p.at(TokenBang) || p.at(TokenPlus) {
		pos := p.cur.Pos
		op := p.cur.Kind
		p.advance()

		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}

		return &UnaryExpr{TokenPos: pos, Op: op, Operand: operand}, nil
	}

	return p.parsePrimary()
}

func (p *parserState) parsePrimary() (ConstExpr, error) {
	switch p.cur.Kind {
	case TokenIntLiteral:
		tok := p.advance()
		return &IntegerLiteral{TokenPos: tok.Pos, Value: tok.Value}, nil

	case TokenFloatLiteral:
		tok := p.advance()
		return &FloatLiteral{TokenPos: tok.Pos, Value: tok.Value}, nil

	case TokenStringLiteral:
		tok := p.advance()
		return &StringLiteralExpr{TokenPos: tok.Pos, Value: tok.Value}, nil

	case TokenCharLiteral:
		tok := p.advance()
		return &CharLiteralExpr{TokenPos: tok.Pos, Value: tok.Value}, nil

	case TokenTrue:
		tok := p.advance()
		return &BoolLiteral{TokenPos: tok.Pos, Value: true}, nil

	case TokenFalse:
		tok := p.advance()
		return &BoolLiteral{TokenPos: tok.Pos, Value: false}, nil

	case TokenNull:
		tok := p.advance()
		return &NullLiteral{TokenPos: tok.Pos}, nil

	case TokenIdent:
		tok := p.advance()
		name := tok.Value

		// Handle dotted names for qualified enum references.
		for p.at(TokenDot) {
			p.advance()
			part, err := p.expect(TokenIdent)
			if err != nil {
				return nil, err
			}
			name += "." + part.Value
		}

		return &IdentExpr{TokenPos: tok.Pos, Name: name}, nil

	case TokenLParen:
		p.advance()
		expr, err := p.parseConstExpr()
		if err != nil {
			return nil, err
		}

		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return expr, nil

	case TokenLBrace:
		// Array/list initializer: { expr, expr, ... }
		tok := p.advance()
		var elements []ConstExpr

		for !p.at(TokenRBrace) && !p.at(TokenEOF) {
			if len(elements) > 0 {
				if _, err := p.expect(TokenComma); err != nil {
					return nil, err
				}

				// Trailing comma.
				if p.at(TokenRBrace) {
					break
				}
			}

			elem, err := p.parseConstExpr()
			if err != nil {
				return nil, err
			}
			elements = append(elements, elem)
		}

		if _, err := p.expect(TokenRBrace); err != nil {
			return nil, err
		}

		// Represent as nested binary expressions with comma for simplicity;
		// or just return the first element for single-element initializers.
		// For now, represent as a special case: use the first element if single,
		// otherwise wrap in a list-like construct via IdentExpr placeholder.
		if len(elements) == 0 {
			return &IdentExpr{TokenPos: tok.Pos, Name: "{}"}, nil
		}
		if len(elements) == 1 {
			return elements[0], nil
		}

		// Build a left-associative chain. The evaluator doesn't need to handle
		// multi-element initializers for AIDL constant evaluation, so this is
		// sufficient for the parser structure.
		result := elements[0]
		for i := 1; i < len(elements); i++ {
			result = &BinaryExpr{
				TokenPos: tok.Pos,
				Op:       TokenComma,
				Left:     result,
				Right:    elements[i],
			}
		}
		return result, nil

	default:
		return nil, fmt.Errorf(
			"%s: expected constant expression, got %s (%q)",
			p.cur.Pos, p.cur.Kind, p.cur.Value,
		)
	}
}
