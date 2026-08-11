package syntax

import (
	"github.com/gdamore/tcell/v2"
)

// TokenType classifies a highlighted span.
type TokenType int

const (
	TPlain TokenType = iota
	TComment
	TString
	TNumber
	TKeyword
	TType
	TBuiltin
	TConstant
	TFunction
	TPreproc
	TDecorator
	TVariable
	TOperator
	TTag
	TAttribute
	TCodeBlock
	THeading
	TLiteral
	TError
)

func (t TokenType) String() string {
	switch t {
	case TComment:
		return "comment"
	case TString:
		return "string"
	case TNumber:
		return "number"
	case TKeyword:
		return "keyword"
	case TType:
		return "type"
	case TBuiltin:
		return "builtin"
	case TConstant:
		return "constant"
	case TFunction:
		return "function"
	case TPreproc:
		return "preproc"
	case TDecorator:
		return "decorator"
	case TVariable:
		return "variable"
	case TOperator:
		return "operator"
	case TTag:
		return "tag"
	case TAttribute:
		return "attribute"
	case TCodeBlock:
		return "codeblock"
	case THeading:
		return "heading"
	case TLiteral:
		return "literal"
	case TError:
		return "error"
	}
	return "plain"
}

// Span is a highlighted range measured in runes.
type Span struct {
	Start, Len int
	Type       TokenType
}

// Theme maps token types to terminal styles.
type Theme struct {
	Plain, Comment, String, Number, Keyword, Type, Builtin, Constant, Function, Preproc, Decorator, Variable, Operator, Tag, Attribute, CodeBlock, Heading, Literal, Error tcell.Style
}

// Style returns the style for a token type.
func (t *Theme) Style(tt TokenType) tcell.Style {
	switch tt {
	case TComment:
		return t.Comment
	case TString:
		return t.String
	case TNumber:
		return t.Number
	case TKeyword:
		return t.Keyword
	case TType:
		return t.Type
	case TBuiltin:
		return t.Builtin
	case TConstant:
		return t.Constant
	case TFunction:
		return t.Function
	case TPreproc:
		return t.Preproc
	case TDecorator:
		return t.Decorator
	case TVariable:
		return t.Variable
	case TOperator:
		return t.Operator
	case TTag:
		return t.Tag
	case TAttribute:
		return t.Attribute
	case TCodeBlock:
		return t.CodeBlock
	case THeading:
		return t.Heading
	case TLiteral:
		return t.Literal
	case TError:
		return t.Error
	}
	return t.Plain
}

// Default returns the built-in dark theme (One-Dark flavored).
func DefaultTheme() *Theme {
	return &Theme{
		Plain:     tcell.StyleDefault.Foreground(tcell.NewHexColor(0xABB2BF)),
		Comment:   tcell.StyleDefault.Foreground(tcell.NewHexColor(0x5C6370)).Italic(true),
		String:    tcell.StyleDefault.Foreground(tcell.NewHexColor(0x98C379)),
		Number:    tcell.StyleDefault.Foreground(tcell.NewHexColor(0xD19A66)),
		Keyword:   tcell.StyleDefault.Foreground(tcell.NewHexColor(0xC678DD)).Bold(true),
		Type:      tcell.StyleDefault.Foreground(tcell.NewHexColor(0x56B6C2)),
		Builtin:   tcell.StyleDefault.Foreground(tcell.NewHexColor(0x61AFEF)),
		Function:  tcell.StyleDefault.Foreground(tcell.NewHexColor(0xE5C07B)),
		Constant:  tcell.StyleDefault.Foreground(tcell.NewHexColor(0xE06C75)),
		Preproc:   tcell.StyleDefault.Foreground(tcell.NewHexColor(0xC678DD)),
		Decorator: tcell.StyleDefault.Foreground(tcell.NewHexColor(0x61AFEF)),
		Variable:  tcell.StyleDefault.Foreground(tcell.NewHexColor(0xD3869B)),
		Operator:  tcell.StyleDefault.Foreground(tcell.NewHexColor(0xABB2BF)),
		Tag:       tcell.StyleDefault.Foreground(tcell.NewHexColor(0xE06C75)),
		Attribute: tcell.StyleDefault.Foreground(tcell.NewHexColor(0xD19A66)),
		CodeBlock: tcell.StyleDefault.Foreground(tcell.NewHexColor(0x56B6C2)),
		Heading:   tcell.StyleDefault.Foreground(tcell.NewHexColor(0xE5C07B)).Bold(true),
		Literal:   tcell.StyleDefault.Foreground(tcell.NewHexColor(0x98C379)),
		Error:     tcell.StyleDefault.Foreground(tcell.NewHexColor(0xFF5555)).Bold(true),
	}
}
