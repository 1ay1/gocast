package vibe

import (
	"fmt"
	"os"
	"strconv"
)

// ParseError represents a parsing error with location information
type ParseError struct {
	Message string
	Line    int
	Column  int
}

func (e ParseError) Error() string {
	return fmt.Sprintf("parse error at line %d, column %d: %s", e.Line, e.Column, e.Message)
}

// Parser parses VIBE configuration files
type Parser struct {
	lexer        *Lexer
	currentToken Token
	peekToken    Token
	errors       []ParseError
}

// NewParser creates a new parser for the given input
func NewParser(input string) *Parser {
	p := &Parser{
		lexer:  NewLexer(input),
		errors: make([]ParseError, 0),
	}
	// Read two tokens to initialize current and peek
	p.nextToken()
	p.nextToken()
	return p
}

// nextToken advances to the next token
func (p *Parser) nextToken() {
	p.currentToken = p.peekToken
	p.peekToken = p.lexer.NextToken()
}

// skipNewlines skips any newline tokens
func (p *Parser) skipNewlines() {
	for p.currentToken.Type == TokenNewline || p.currentToken.Type == TokenComment {
		p.nextToken()
	}
}

// addError adds a parse error
func (p *Parser) addError(format string, args ...interface{}) {
	p.errors = append(p.errors, ParseError{
		Message: fmt.Sprintf(format, args...),
		Line:    p.currentToken.Line,
		Column:  p.currentToken.Column,
	})
}

// Errors returns all parsing errors
func (p *Parser) Errors() []ParseError {
	return p.errors
}

// HasErrors returns true if there are parsing errors
func (p *Parser) HasErrors() bool {
	return len(p.errors) > 0
}

// Parse parses the input and returns the root value
func (p *Parser) Parse() (*Value, error) {
	obj := NewObject()

	p.skipNewlines()

	for p.currentToken.Type != TokenEOF {
		if p.currentToken.Type == TokenError {
			p.addError("lexer error: %s", p.currentToken.Value)
			p.nextToken()
			continue
		}

		if p.currentToken.Type == TokenNewline || p.currentToken.Type == TokenComment {
			p.nextToken()
			continue
		}

		if p.currentToken.Type != TokenIdentifier {
			p.addError("expected identifier, got %s", p.currentToken.Type)
			p.nextToken()
			continue
		}

		key := p.currentToken.Value
		keyLine := p.currentToken.Line
		keyColumn := p.currentToken.Column
		p.nextToken()

		p.skipNewlines()

		value := p.parseValue()
		if value != nil {
			value.Line = keyLine
			value.Column = keyColumn
			obj.Set(key, value)
		}

		p.skipNewlines()
	}

	if p.HasErrors() {
		return nil, p.errors[0]
	}

	return NewObjectValue(obj), nil
}

// parseValue parses a value (object, array, or scalar)
func (p *Parser) parseValue() *Value {
	switch p.currentToken.Type {
	case TokenLeftBrace:
		return p.parseObject()
	case TokenLeftBracket:
		return p.parseArray()
	case TokenString:
		val := NewStringValue(p.currentToken.Value)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	case TokenInt:
		i, err := strconv.ParseInt(p.currentToken.Value, 10, 64)
		if err != nil {
			p.addError("invalid integer: %s", p.currentToken.Value)
			p.nextToken()
			return nil
		}
		val := NewIntValue(i)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	case TokenFloat:
		f, err := strconv.ParseFloat(p.currentToken.Value, 64)
		if err != nil {
			p.addError("invalid float: %s", p.currentToken.Value)
			p.nextToken()
			return nil
		}
		val := NewFloatValue(f)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	case TokenBool:
		b := p.currentToken.Value == "true"
		val := NewBoolValue(b)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	case TokenIdentifier:
		// Treat as unquoted string
		val := NewStringValue(p.currentToken.Value)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	case TokenNewline, TokenEOF:
		// No value
		return nil
	default:
		p.addError("unexpected token: %s", p.currentToken.Type)
		p.nextToken()
		return nil
	}
}

// parseObject parses an object { ... }
func (p *Parser) parseObject() *Value {
	obj := NewObject()
	startLine := p.currentToken.Line
	startColumn := p.currentToken.Column

	p.nextToken() // consume {
	p.skipNewlines()

	for p.currentToken.Type != TokenRightBrace && p.currentToken.Type != TokenEOF {
		if p.currentToken.Type == TokenError {
			p.addError("lexer error: %s", p.currentToken.Value)
			p.nextToken()
			continue
		}

		if p.currentToken.Type == TokenNewline || p.currentToken.Type == TokenComment {
			p.nextToken()
			continue
		}

		if p.currentToken.Type != TokenIdentifier {
			p.addError("expected identifier in object, got %s", p.currentToken.Type)
			p.nextToken()
			continue
		}

		key := p.currentToken.Value
		keyLine := p.currentToken.Line
		keyColumn := p.currentToken.Column
		p.nextToken()

		p.skipNewlines()

		value := p.parseValue()
		if value != nil {
			value.Line = keyLine
			value.Column = keyColumn
			obj.Set(key, value)
		}

		p.skipNewlines()
	}

	if p.currentToken.Type != TokenRightBrace {
		p.addError("unclosed object starting at line %d", startLine)
	} else {
		p.nextToken() // consume }
	}

	val := NewObjectValue(obj)
	val.Line = startLine
	val.Column = startColumn
	return val
}

// parseArray parses an array [ ... ]
func (p *Parser) parseArray() *Value {
	arr := make([]*Value, 0)
	startLine := p.currentToken.Line
	startColumn := p.currentToken.Column

	p.nextToken() // consume [
	p.skipNewlines()

	for p.currentToken.Type != TokenRightBracket && p.currentToken.Type != TokenEOF {
		if p.currentToken.Type == TokenError {
			p.addError("lexer error: %s", p.currentToken.Value)
			p.nextToken()
			continue
		}

		if p.currentToken.Type == TokenNewline || p.currentToken.Type == TokenComment {
			p.nextToken()
			continue
		}

		// Check for nested objects/arrays (not allowed in VIBE)
		if p.currentToken.Type == TokenLeftBrace {
			p.addError("objects cannot be placed inside arrays. Use named objects instead")
			p.skipUntilBrace()
			continue
		}

		if p.currentToken.Type == TokenLeftBracket {
			p.addError("arrays cannot be nested inside other arrays")
			p.skipUntilBracket()
			continue
		}

		value := p.parseScalarValue()
		if value != nil {
			arr = append(arr, value)
		}

		p.skipNewlines()
	}

	if p.currentToken.Type != TokenRightBracket {
		p.addError("unclosed array starting at line %d", startLine)
	} else {
		p.nextToken() // consume ]
	}

	val := NewArrayValue(arr)
	val.Line = startLine
	val.Column = startColumn
	return val
}

// parseScalarValue parses only scalar values (for arrays)
func (p *Parser) parseScalarValue() *Value {
	switch p.currentToken.Type {
	case TokenString:
		val := NewStringValue(p.currentToken.Value)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	case TokenInt:
		i, err := strconv.ParseInt(p.currentToken.Value, 10, 64)
		if err != nil {
			p.addError("invalid integer: %s", p.currentToken.Value)
			p.nextToken()
			return nil
		}
		val := NewIntValue(i)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	case TokenFloat:
		f, err := strconv.ParseFloat(p.currentToken.Value, 64)
		if err != nil {
			p.addError("invalid float: %s", p.currentToken.Value)
			p.nextToken()
			return nil
		}
		val := NewFloatValue(f)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	case TokenBool:
		b := p.currentToken.Value == "true"
		val := NewBoolValue(b)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	case TokenIdentifier:
		// Treat as unquoted string
		val := NewStringValue(p.currentToken.Value)
		val.Line = p.currentToken.Line
		val.Column = p.currentToken.Column
		p.nextToken()
		return val
	default:
		p.addError("unexpected token in array: %s", p.currentToken.Type)
		p.nextToken()
		return nil
	}
}

// skipUntilBrace skips tokens until a matching } is found
func (p *Parser) skipUntilBrace() {
	depth := 1
	p.nextToken() // skip {
	for depth > 0 && p.currentToken.Type != TokenEOF {
		if p.currentToken.Type == TokenLeftBrace {
			depth++
		} else if p.currentToken.Type == TokenRightBrace {
			depth--
		}
		p.nextToken()
	}
}

// skipUntilBracket skips tokens until a matching ] is found
func (p *Parser) skipUntilBracket() {
	depth := 1
	p.nextToken() // skip [
	for depth > 0 && p.currentToken.Type != TokenEOF {
		if p.currentToken.Type == TokenLeftBracket {
			depth++
		} else if p.currentToken.Type == TokenRightBracket {
			depth--
		}
		p.nextToken()
	}
}

// ParseString parses a VIBE string and returns the root value
func ParseString(input string) (*Value, error) {
	parser := NewParser(input)
	return parser.Parse()
}

// ParseFile parses a VIBE file and returns the root value
func ParseFile(filename string) (*Value, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return ParseString(string(data))
}

// MustParseString parses a VIBE string and panics on error
func MustParseString(input string) *Value {
	v, err := ParseString(input)
	if err != nil {
		panic(err)
	}
	return v
}

// MustParseFile parses a VIBE file and panics on error
func MustParseFile(filename string) *Value {
	v, err := ParseFile(filename)
	if err != nil {
		panic(err)
	}
	return v
}
