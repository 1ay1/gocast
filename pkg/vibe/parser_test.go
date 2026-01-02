package vibe

import (
	"testing"
)

func TestParseSimpleAssignment(t *testing.T) {
	input := `
name "My App"
version 1.0
enabled true
count 42
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if v.GetString("name") != "My App" {
		t.Errorf("expected name='My App', got '%s'", v.GetString("name"))
	}

	if v.GetFloat("version") != 1.0 {
		t.Errorf("expected version=1.0, got %f", v.GetFloat("version"))
	}

	if v.GetBool("enabled") != true {
		t.Errorf("expected enabled=true, got %v", v.GetBool("enabled"))
	}

	if v.GetInt("count") != 42 {
		t.Errorf("expected count=42, got %d", v.GetInt("count"))
	}
}

func TestParseObject(t *testing.T) {
	input := `
server {
	host localhost
	port 8080
}
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if v.GetString("server.host") != "localhost" {
		t.Errorf("expected server.host='localhost', got '%s'", v.GetString("server.host"))
	}

	if v.GetInt("server.port") != 8080 {
		t.Errorf("expected server.port=8080, got %d", v.GetInt("server.port"))
	}
}

func TestParseNestedObject(t *testing.T) {
	input := `
app {
	name "MyApp"
	database {
		host db.example.com
		port 5432
		ssl {
			enabled true
			cert /path/to/cert
		}
	}
}
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if v.GetString("app.name") != "MyApp" {
		t.Errorf("expected app.name='MyApp', got '%s'", v.GetString("app.name"))
	}

	if v.GetString("app.database.host") != "db.example.com" {
		t.Errorf("expected app.database.host='db.example.com', got '%s'", v.GetString("app.database.host"))
	}

	if v.GetInt("app.database.port") != 5432 {
		t.Errorf("expected app.database.port=5432, got %d", v.GetInt("app.database.port"))
	}

	if v.GetBool("app.database.ssl.enabled") != true {
		t.Errorf("expected app.database.ssl.enabled=true, got %v", v.GetBool("app.database.ssl.enabled"))
	}

	if v.GetString("app.database.ssl.cert") != "/path/to/cert" {
		t.Errorf("expected app.database.ssl.cert='/path/to/cert', got '%s'", v.GetString("app.database.ssl.cert"))
	}
}

func TestParseArray(t *testing.T) {
	input := `
ports [8080 8081 8082]
hosts [
	server1.com
	server2.com
	server3.com
]
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	ports := v.GetArray("ports")
	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(ports))
	}
	if ports[0].Int != 8080 {
		t.Errorf("expected ports[0]=8080, got %d", ports[0].Int)
	}
	if ports[1].Int != 8081 {
		t.Errorf("expected ports[1]=8081, got %d", ports[1].Int)
	}
	if ports[2].Int != 8082 {
		t.Errorf("expected ports[2]=8082, got %d", ports[2].Int)
	}

	hosts := v.GetStringArray("hosts")
	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(hosts))
	}
	if hosts[0] != "server1.com" {
		t.Errorf("expected hosts[0]='server1.com', got '%s'", hosts[0])
	}
	if hosts[1] != "server2.com" {
		t.Errorf("expected hosts[1]='server2.com', got '%s'", hosts[1])
	}
	if hosts[2] != "server3.com" {
		t.Errorf("expected hosts[2]='server3.com', got '%s'", hosts[2])
	}
}

func TestParseComments(t *testing.T) {
	input := `
# This is a comment
name "Test"  # Inline comment
# Another comment
value 123
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if v.GetString("name") != "Test" {
		t.Errorf("expected name='Test', got '%s'", v.GetString("name"))
	}

	if v.GetInt("value") != 123 {
		t.Errorf("expected value=123, got %d", v.GetInt("value"))
	}
}

func TestParseEscapeSequences(t *testing.T) {
	input := `
message "Hello\nWorld"
path "C:\\Users\\Name"
quote "He said \"Hello\""
tab "col1\tcol2"
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if v.GetString("message") != "Hello\nWorld" {
		t.Errorf("expected message='Hello\\nWorld', got '%s'", v.GetString("message"))
	}

	if v.GetString("path") != "C:\\Users\\Name" {
		t.Errorf("expected path='C:\\Users\\Name', got '%s'", v.GetString("path"))
	}

	if v.GetString("quote") != "He said \"Hello\"" {
		t.Errorf("expected quote='He said \"Hello\"', got '%s'", v.GetString("quote"))
	}

	if v.GetString("tab") != "col1\tcol2" {
		t.Errorf("expected tab='col1\\tcol2', got '%s'", v.GetString("tab"))
	}
}

func TestParseNumbers(t *testing.T) {
	input := `
positive 42
negative -17
float_val 3.14
negative_float -0.5
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if v.GetInt("positive") != 42 {
		t.Errorf("expected positive=42, got %d", v.GetInt("positive"))
	}

	if v.GetInt("negative") != -17 {
		t.Errorf("expected negative=-17, got %d", v.GetInt("negative"))
	}

	if v.GetFloat("float_val") != 3.14 {
		t.Errorf("expected float_val=3.14, got %f", v.GetFloat("float_val"))
	}

	if v.GetFloat("negative_float") != -0.5 {
		t.Errorf("expected negative_float=-0.5, got %f", v.GetFloat("negative_float"))
	}
}

func TestParseBooleans(t *testing.T) {
	input := `
enabled true
disabled false
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if v.GetBool("enabled") != true {
		t.Errorf("expected enabled=true, got %v", v.GetBool("enabled"))
	}

	if v.GetBool("disabled") != false {
		t.Errorf("expected disabled=false, got %v", v.GetBool("disabled"))
	}
}

func TestParseEmptyArray(t *testing.T) {
	input := `items []`

	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	items := v.GetArray("items")
	if len(items) != 0 {
		t.Errorf("expected empty array, got %d items", len(items))
	}
}

func TestParseEmptyObject(t *testing.T) {
	input := `config {}`

	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	obj := v.GetObject("config")
	if obj == nil {
		t.Fatal("expected object, got nil")
	}

	if obj.Len() != 0 {
		t.Errorf("expected empty object, got %d keys", obj.Len())
	}
}

func TestPathAccess(t *testing.T) {
	input := `
servers {
	primary {
		host server1.com
		port 8080
	}
	secondary {
		host server2.com
		port 9090
	}
}
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	// Test deep path access
	if v.GetString("servers.primary.host") != "server1.com" {
		t.Errorf("expected servers.primary.host='server1.com', got '%s'", v.GetString("servers.primary.host"))
	}

	if v.GetInt("servers.secondary.port") != 9090 {
		t.Errorf("expected servers.secondary.port=9090, got %d", v.GetInt("servers.secondary.port"))
	}

	// Test non-existent path
	if v.GetString("servers.tertiary.host") != "" {
		t.Errorf("expected empty string for non-existent path, got '%s'", v.GetString("servers.tertiary.host"))
	}
}

func TestArrayIndexAccess(t *testing.T) {
	input := `
numbers [10 20 30 40 50]
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	// Test array index access via GetPath
	val := v.GetPath("numbers[0]")
	if val == nil || val.Int != 10 {
		t.Errorf("expected numbers[0]=10, got %v", val)
	}

	val = v.GetPath("numbers[2]")
	if val == nil || val.Int != 30 {
		t.Errorf("expected numbers[2]=30, got %v", val)
	}

	val = v.GetPath("numbers[4]")
	if val == nil || val.Int != 50 {
		t.Errorf("expected numbers[4]=50, got %v", val)
	}

	// Out of bounds should return nil
	val = v.GetPath("numbers[10]")
	if val != nil {
		t.Errorf("expected nil for out of bounds, got %v", val)
	}
}

func TestDefaultValues(t *testing.T) {
	input := `
name "Test"
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	// Test default values for non-existent keys
	if v.GetStringDefault("missing", "default") != "default" {
		t.Errorf("expected 'default', got '%s'", v.GetStringDefault("missing", "default"))
	}

	if v.GetIntDefault("missing", 100) != 100 {
		t.Errorf("expected 100, got %d", v.GetIntDefault("missing", 100))
	}

	if v.GetFloatDefault("missing", 3.14) != 3.14 {
		t.Errorf("expected 3.14, got %f", v.GetFloatDefault("missing", 3.14))
	}

	if v.GetBoolDefault("missing", true) != true {
		t.Errorf("expected true, got %v", v.GetBoolDefault("missing", true))
	}
}

func TestUnquotedStrings(t *testing.T) {
	input := `
hostname server.example.com
path /usr/local/bin
email admin@example.com
version 2.1.0-beta
url https://api.example.com/v1
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"hostname", "server.example.com"},
		{"path", "/usr/local/bin"},
		{"email", "admin@example.com"},
		{"version", "2.1.0-beta"},
		{"url", "https://api.example.com/v1"},
	}

	for _, tt := range tests {
		got := v.GetString(tt.key)
		if got != tt.expected {
			t.Errorf("expected %s='%s', got '%s'", tt.key, tt.expected, got)
		}
	}
}

func TestMixedArray(t *testing.T) {
	input := `
mixed [42 "hello" true 3.14]
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	arr := v.GetArray("mixed")
	if len(arr) != 4 {
		t.Fatalf("expected 4 items, got %d", len(arr))
	}

	if arr[0].Type != TypeInt || arr[0].Int != 42 {
		t.Errorf("expected arr[0]=42 (int), got %v", arr[0])
	}

	if arr[1].Type != TypeString || arr[1].String != "hello" {
		t.Errorf("expected arr[1]='hello' (string), got %v", arr[1])
	}

	if arr[2].Type != TypeBool || arr[2].Bool != true {
		t.Errorf("expected arr[2]=true (bool), got %v", arr[2])
	}

	if arr[3].Type != TypeFloat || arr[3].Float != 3.14 {
		t.Errorf("expected arr[3]=3.14 (float), got %v", arr[3])
	}
}

func TestComplexConfig(t *testing.T) {
	input := `
# GoCast-like configuration
server {
	hostname localhost
	port 8000
	ssl {
		enabled true
		cert /etc/ssl/cert.pem
	}
}

auth {
	source_password hackme
	admin_user admin
	admin_password secret
}

limits {
	max_clients 100
	max_sources 10
}

mounts {
	live {
		max_listeners 50
		genre "Music"
		public true
	}
	radio {
		max_listeners 100
		genre "Talk"
		public false
	}
}

features [auth logging metrics]
`
	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	// Test server config
	if v.GetString("server.hostname") != "localhost" {
		t.Errorf("expected server.hostname='localhost'")
	}
	if v.GetInt("server.port") != 8000 {
		t.Errorf("expected server.port=8000")
	}
	if v.GetBool("server.ssl.enabled") != true {
		t.Errorf("expected server.ssl.enabled=true")
	}

	// Test auth config
	if v.GetString("auth.admin_user") != "admin" {
		t.Errorf("expected auth.admin_user='admin'")
	}

	// Test limits
	if v.GetInt("limits.max_clients") != 100 {
		t.Errorf("expected limits.max_clients=100")
	}

	// Test nested mounts
	if v.GetInt("mounts.live.max_listeners") != 50 {
		t.Errorf("expected mounts.live.max_listeners=50")
	}
	if v.GetBool("mounts.radio.public") != false {
		t.Errorf("expected mounts.radio.public=false")
	}

	// Test array
	features := v.GetStringArray("features")
	if len(features) != 3 {
		t.Fatalf("expected 3 features, got %d", len(features))
	}
	if features[0] != "auth" || features[1] != "logging" || features[2] != "metrics" {
		t.Errorf("unexpected features: %v", features)
	}
}

func TestLexerTokens(t *testing.T) {
	input := `name "value" 123 true { } [ ]`

	lexer := NewLexer(input)
	tokens := lexer.Tokenize()

	expectedTypes := []TokenType{
		TokenIdentifier,   // name
		TokenString,       // "value"
		TokenInt,          // 123
		TokenBool,         // true
		TokenLeftBrace,    // {
		TokenRightBrace,   // }
		TokenLeftBracket,  // [
		TokenRightBracket, // ]
		TokenEOF,
	}

	if len(tokens) != len(expectedTypes) {
		t.Fatalf("expected %d tokens, got %d", len(expectedTypes), len(tokens))
	}

	for i, expected := range expectedTypes {
		if tokens[i].Type != expected {
			t.Errorf("token %d: expected %s, got %s", i, expected, tokens[i].Type)
		}
	}
}

func TestParseError(t *testing.T) {
	// Unclosed object
	input := `server { host localhost`

	_, err := ParseString(input)
	if err == nil {
		t.Error("expected parse error for unclosed object")
	}
}

func TestEmptyInput(t *testing.T) {
	input := ``

	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if v == nil {
		t.Fatal("expected non-nil value")
	}

	if v.Type != TypeObject {
		t.Errorf("expected object type, got %s", v.Type)
	}
}

func TestWhitespaceOnly(t *testing.T) {
	input := `


	`

	v, err := ParseString(input)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if v == nil {
		t.Fatal("expected non-nil value")
	}
}
