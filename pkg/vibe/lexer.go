package vibe

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenType represents the type of a lexical token
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenError
	TokenIdentifier
	TokenString
	TokenInt
	TokenFloat
	TokenBool
	TokenLeftBrace    // {
	TokenRightBrace   // }
	TokenLeftBracket  // [
	TokenRightBracket // ]
	TokenNewline
	TokenComment
)

func (t TokenType) String() string {
	switch t {
	case TokenEOF:
		return "EOF"
	case TokenError:
		return "ERROR"
	case TokenIdentifier:
		return "IDENTIFIER"
	case TokenString:
		return "STRING"
	case TokenInt:
		return "INT"
	case TokenFloat:
		return "FLOAT"
	case TokenBool:
		return "BOOL"
	case TokenLeftBrace:
		return "LEFT_BRACE"
	case TokenRightBrace:
		return "RIGHT_BRACE"
	case TokenLeftBracket:
		return "LEFT_BRACKET"
	case TokenRightBracket:
		return "RIGHT_BRACKET"
	case TokenNewline:
		return "NEWLINE"
	case TokenComment:
		return "COMMENT"
	default:
		return "UNKNOWN"
	}
}

// Token represents a lexical token
type Token struct {
	Type   TokenType
	Value  string
	Line   int
	Column int
}

func (t Token) String() string {
	return fmt.Sprintf("Token{%s, %q, L%d:C%d}", t.Type, t.Value, t.Line, t.Column)
}

// Lexer tokenizes VIBE input
type Lexer struct {
	input   string
	pos     int  // current position in input
	readPos int  // reading position (after current char)
	ch      rune // current character
	line    int
	column  int
}

// NewLexer creates a new lexer for the given input
func NewLexer(input string) *Lexer {
	l := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	l.readChar()
	return l
}

// readChar reads the next character
func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0 // EOF
	} else {
		r, size := utf8.DecodeRuneInString(l.input[l.readPos:])
		l.ch = r
		l.pos = l.readPos
		l.readPos += size
	}

	if l.ch == '\n' {
		l.line++
		l.column = 0
	} else {
		l.column++
	}
}

// peekChar peeks at the next character without consuming it
func (l *Lexer) peekChar() rune {
	if l.readPos >= len(l.input) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.input[l.readPos:])
	return r
}

// skipWhitespace skips spaces and tabs (not newlines)
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' {
		l.readChar()
	}
}

// NextToken returns the next token from the input
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	tok := Token{
		Line:   l.line,
		Column: l.column,
	}

	switch l.ch {
	case 0:
		tok.Type = TokenEOF
		tok.Value = ""
	case '\n':
		tok.Type = TokenNewline
		tok.Value = "\n"
		l.readChar()
	case '{':
		tok.Type = TokenLeftBrace
		tok.Value = "{"
		l.readChar()
	case '}':
		tok.Type = TokenRightBrace
		tok.Value = "}"
		l.readChar()
	case '[':
		tok.Type = TokenLeftBracket
		tok.Value = "["
		l.readChar()
	case ']':
		tok.Type = TokenRightBracket
		tok.Value = "]"
		l.readChar()
	case '#':
		tok.Type = TokenComment
		tok.Value = l.readComment()
	case '"':
		str, err := l.readQuotedString()
		if err != nil {
			tok.Type = TokenError
			tok.Value = err.Error()
		} else {
			tok.Type = TokenString
			tok.Value = str
		}
	default:
		if isIdentifierStart(l.ch) || isDigit(l.ch) || l.ch == '-' || isUnquotedStringStart(l.ch) {
			return l.readValueToken()
		}
		tok.Type = TokenError
		tok.Value = fmt.Sprintf("unexpected character: %q", l.ch)
		l.readChar()
	}

	return tok
}

// readComment reads a comment (everything after # until newline)
func (l *Lexer) readComment() string {
	var sb strings.Builder
	l.readChar() // skip the #
	for l.ch != '\n' && l.ch != 0 {
		sb.WriteRune(l.ch)
		l.readChar()
	}
	return strings.TrimSpace(sb.String())
}

// readQuotedString reads a quoted string with escape sequence support
func (l *Lexer) readQuotedString() (string, error) {
	var sb strings.Builder
	l.readChar() // skip opening quote

	for {
		if l.ch == 0 {
			return "", fmt.Errorf("unterminated string at line %d", l.line)
		}
		if l.ch == '"' {
			l.readChar() // skip closing quote
			break
		}
		if l.ch == '\\' {
			l.readChar() // skip backslash
			switch l.ch {
			case '"':
				sb.WriteRune('"')
			case '\\':
				sb.WriteRune('\\')
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case 'r':
				sb.WriteRune('\r')
			case 'u':
				// Unicode escape: \uXXXX
				hex := ""
				for i := 0; i < 4; i++ {
					l.readChar()
					if !isHexDigit(l.ch) {
						return "", fmt.Errorf("invalid unicode escape at line %d", l.line)
					}
					hex += string(l.ch)
				}
				var r rune
				fmt.Sscanf(hex, "%x", &r)
				sb.WriteRune(r)
			default:
				return "", fmt.Errorf("invalid escape sequence '\\%c' at line %d", l.ch, l.line)
			}
		} else {
			sb.WriteRune(l.ch)
		}
		l.readChar()
	}

	return sb.String(), nil
}

// readValueToken reads an identifier, number, boolean, or unquoted string
func (l *Lexer) readValueToken() Token {
	tok := Token{
		Line:   l.line,
		Column: l.column,
	}

	startPos := l.pos
	hasDecimal := false
	isNegative := l.ch == '-'
	allDigits := true

	if isNegative {
		l.readChar()
		// Check if next char is a digit for negative numbers
		if !isDigit(l.ch) {
			// Not a number, treat as unquoted string starting with -
			allDigits = false
		}
	}

	// Read until we hit a delimiter
	for !isDelimiter(l.ch) && l.ch != 0 {
		if l.ch == '.' {
			// Check if this looks like a version number (digits.digits.digits)
			// or a domain name, not a float
			if hasDecimal {
				allDigits = false
			}
			hasDecimal = true
		} else if !isDigit(l.ch) {
			allDigits = false
		}
		l.readChar()
	}

	// If we have more than one decimal, it's not a float (e.g., "1.2.3" is a string)
	value := l.input[startPos:l.pos]
	decimalCount := 0
	for _, c := range value {
		if c == '.' {
			decimalCount++
		}
	}
	if decimalCount > 1 {
		allDigits = false
		hasDecimal = false
	}

	// Determine token type
	if value == "true" || value == "false" {
		tok.Type = TokenBool
		tok.Value = value
	} else if allDigits && len(value) > 0 {
		if hasDecimal {
			tok.Type = TokenFloat
		} else {
			tok.Type = TokenInt
		}
		tok.Value = value
	} else if isValidIdentifier(value) {
		tok.Type = TokenIdentifier
		tok.Value = value
	} else if isValidUnquotedString(value) {
		// Unquoted string (paths, URLs, emails, etc.)
		tok.Type = TokenString
		tok.Value = value
	} else {
		tok.Type = TokenError
		tok.Value = fmt.Sprintf("invalid token: %s", value)
	}

	return tok
}

// isIdentifierStart checks if a character can start an identifier
func isIdentifierStart(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

// isIdentifierChar checks if a character can be part of an identifier
func isIdentifierChar(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '-'
}

// isValidIdentifier checks if a string is a valid identifier
func isValidIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	if !isIdentifierStart(r) {
		return false
	}
	for _, ch := range s[1:] {
		if !isIdentifierChar(ch) {
			return false
		}
	}
	return true
}

// isValidUnquotedString checks if a string is a valid unquoted string
func isValidUnquotedString(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Unquoted strings can contain letters, digits, and: _ - . / : @
	for _, ch := range s {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) &&
			ch != '_' && ch != '-' && ch != '.' && ch != '/' && ch != ':' && ch != '@' && ch != '!' {
			return false
		}
	}
	return true
}

// isDigit checks if a character is a digit
func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

// isUnquotedStringStart checks if a character can start an unquoted string
func isUnquotedStringStart(ch rune) bool {
	return ch == '/' || ch == '.' || ch == ':' || ch == '@'
}

// isUnquotedStringChar checks if a character can be part of an unquoted string
func isUnquotedStringChar(ch rune) bool {
	return isIdentifierChar(ch) || ch == '/' || ch == '.' || ch == ':' || ch == '@' || ch == '!'
}

// isHexDigit checks if a character is a hexadecimal digit
func isHexDigit(ch rune) bool {
	return isDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// isDelimiter checks if a character is a token delimiter
func isDelimiter(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' ||
		ch == '{' || ch == '}' || ch == '[' || ch == ']' || ch == '#'
}

// PeekToken returns the next token without consuming it
func (l *Lexer) PeekToken() Token {
	// Save state
	pos := l.pos
	readPos := l.readPos
	ch := l.ch
	line := l.line
	column := l.column

	tok := l.NextToken()

	// Restore state
	l.pos = pos
	l.readPos = readPos
	l.ch = ch
	l.line = line
	l.column = column

	return tok
}

// Tokenize returns all tokens from the input
func (l *Lexer) Tokenize() []Token {
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return tokens
}
