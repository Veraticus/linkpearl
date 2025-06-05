package main

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestStringSliceFlag tests the custom stringSliceFlag type.
func TestStringSliceFlag(t *testing.T) {
	tests := []struct {
		name     string
		initial  []string
		setValue []string
		want     []string
		wantStr  string
	}{
		{
			name:     "empty flag",
			initial:  nil,
			setValue: []string{},
			want:     nil,
			wantStr:  "",
		},
		{
			name:     "single value",
			initial:  nil,
			setValue: []string{"host1:8080"},
			want:     []string{"host1:8080"},
			wantStr:  "host1:8080",
		},
		{
			name:     "multiple values via multiple calls",
			initial:  nil,
			setValue: []string{"host1:8080", "host2:8080"},
			want:     []string{"host1:8080", "host2:8080"},
			wantStr:  "host1:8080,host2:8080",
		},
		{
			name:     "comma-separated values",
			initial:  nil,
			setValue: []string{"host1:8080,host2:8080,host3:8080"},
			want:     []string{"host1:8080", "host2:8080", "host3:8080"},
			wantStr:  "host1:8080,host2:8080,host3:8080",
		},
		{
			name:     "mixed single and comma-separated",
			initial:  nil,
			setValue: []string{"host1:8080", "host2:8080,host3:8080", "host4:8080"},
			want:     []string{"host1:8080", "host2:8080", "host3:8080", "host4:8080"},
			wantStr:  "host1:8080,host2:8080,host3:8080,host4:8080",
		},
		{
			name:     "with spaces",
			initial:  nil,
			setValue: []string{"  host1:8080  ", " host2:8080 , host3:8080 "},
			want:     []string{"host1:8080", "host2:8080", "host3:8080"},
			wantStr:  "host1:8080,host2:8080,host3:8080",
		},
		{
			name:     "empty values filtered",
			initial:  nil,
			setValue: []string{"host1:8080,,host2:8080", "", ",,,", "host3:8080"},
			want:     []string{"host1:8080", "host2:8080", "host3:8080"},
			wantStr:  "host1:8080,host2:8080,host3:8080",
		},
		{
			name:     "append to existing",
			initial:  []string{"existing:8080"},
			setValue: []string{"new1:8080", "new2:8080"},
			want:     []string{"existing:8080", "new1:8080", "new2:8080"},
			wantStr:  "existing:8080,new1:8080,new2:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create flag with initial values
			flag := stringSliceFlag(tt.initial)

			// Set values
			for _, v := range tt.setValue {
				if err := flag.Set(v); err != nil {
					t.Errorf("Set(%q) error = %v", v, err)
				}
			}

			// Check Get()
			got := flag.Get()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get() = %v, want %v", got, tt.want)
			}

			// Check String()
			gotStr := flag.String()
			if gotStr != tt.wantStr {
				t.Errorf("String() = %q, want %q", gotStr, tt.wantStr)
			}
		})
	}
}

// TestStringSliceFlagNil tests nil handling.
func TestStringSliceFlagNil(t *testing.T) {
	var flag *stringSliceFlag

	// String() on nil should return empty string
	if flag.String() != "" {
		t.Errorf("nil.String() = %q, want empty", flag.String())
	}

	// Get() on nil should return nil
	if flag.Get() != nil {
		t.Errorf("nil.Get() = %v, want nil", flag.Get())
	}
}

// TestStringSliceFlagWithFlagPackage tests integration with the flag package.
func TestStringSliceFlagWithFlagPackage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "single flag occurrence",
			args: []string{"-join", "host1:8080"},
			want: []string{"host1:8080"},
		},
		{
			name: "multiple flag occurrences",
			args: []string{"-join", "host1:8080", "-join", "host2:8080"},
			want: []string{"host1:8080", "host2:8080"},
		},
		{
			name: "comma-separated in single flag",
			args: []string{"-join", "host1:8080,host2:8080,host3:8080"},
			want: []string{"host1:8080", "host2:8080", "host3:8080"},
		},
		{
			name: "mixed usage",
			args: []string{"-join", "host1:8080", "-join", "host2:8080,host3:8080", "-other", "value", "-join", "host4:8080"},
			want: []string{"host1:8080", "host2:8080", "host3:8080", "host4:8080"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new flag set for each test
			fs := flag.NewFlagSet("test", flag.ContinueOnError)

			// Define flags
			var joinAddrs stringSliceFlag
			var otherFlag string
			fs.Var(&joinAddrs, "join", "Address to join")
			fs.StringVar(&otherFlag, "other", "", "Other flag")

			// Parse
			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			// Check result
			got := joinAddrs.Get()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("after Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValidateAddress tests the address validation function.
func TestValidateAddress(t *testing.T) {
	tests := []struct {
		addr    string
		wantErr bool
		errMsg  string
	}{
		// Valid addresses
		{
			addr:    "localhost:8080",
			wantErr: false,
		},
		{
			addr:    "example.com:8080",
			wantErr: false,
		},
		{
			addr:    "192.168.1.1:8080",
			wantErr: false,
		},
		{
			addr:    "[::1]:8080",
			wantErr: false,
		},
		{
			addr:    ":8080",
			wantErr: false,
		},
		{
			addr:    "host.with-dash.com:8080",
			wantErr: false,
		},
		{
			addr:    "host_with_underscore:8080",
			wantErr: false,
		},
		{
			addr:    "host:65535",
			wantErr: false,
		},

		// Invalid addresses
		{
			addr:    "",
			wantErr: true,
			errMsg:  "empty address",
		},
		{
			addr:    "no-port",
			wantErr: true,
			errMsg:  "address should be in format host:port or :port",
		},
		{
			addr:    "just-a-hostname",
			wantErr: true,
			errMsg:  "address should be in format host:port or :port",
		},
		{
			addr:    "  ",
			wantErr: true,
			errMsg:  "address should be in format host:port or :port",
		},
		{
			addr:    "host:",
			wantErr: false, // This is valid - port will be validated later
		},
		{
			addr:    "multiple:colons:here",
			wantErr: false, // We don't validate this deeply
		},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := validateAddress(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAddress(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
				return
			}

			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("validateAddress(%q) error message = %q, want %q", tt.addr, err.Error(), tt.errMsg)
			}
		})
	}
}

// TestFlagUsage tests the custom usage function.
func TestFlagUsage(t *testing.T) {
	// Capture original stderr
	oldStderr := os.Stderr
	defer func() { os.Stderr = oldStderr }()

	// Create pipe to capture output
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	// Create flag set with custom usage
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	// Set up flags similar to main
	var (
		secret      string
		nodeID      string
		listen      string
		joinAddrs   stringSliceFlag
		verbose     bool
		showVersion bool
	)

	fs.StringVar(&secret, "secret", "", "Shared secret for linkshell (required)")
	fs.StringVar(&nodeID, "node-id", "auto-generated", "Node identifier (auto-generated if not set)")
	fs.StringVar(&listen, "listen", "", "Listen address (e.g., :8080 or 0.0.0.0:8080)")
	fs.Var(&joinAddrs, "join", "Address to join (can be repeated or comma-separated)")
	fs.BoolVar(&verbose, "v", false, "Enable verbose logging")
	fs.BoolVar(&showVersion, "version", false, "Show version information")

	// Set custom usage
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "linkpearl - Secure P2P clipboard synchronization\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  test [options]\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  # Start a full node (accepts connections)\n")
		fmt.Fprintf(os.Stderr, "  test --secret mysecret --listen :8080\n\n")
		fmt.Fprintf(os.Stderr, "  # Start a client node (outbound only)\n")
		fmt.Fprintf(os.Stderr, "  test --secret mysecret --join server.example.com:8080\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	// Call usage
	fs.Usage()

	// Close writer and read output
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	output := make([]byte, 4096)
	n, _ := r.Read(output)
	outputStr := string(output[:n])

	// Check output contains expected elements
	expectedStrings := []string{
		"linkpearl - Secure P2P clipboard synchronization",
		"Usage:",
		"Examples:",
		"Start a full node",
		"Start a client node",
		"Options:",
		"--secret",
		"--join",
		"--listen",
	}

	for _, expected := range expectedStrings {
		if !contains(outputStr, expected) {
			t.Errorf("usage output missing %q\nGot:\n%s", expected, outputStr)
		}
	}
}

// TestFlagPrecedence tests that flags take precedence over defaults.
func TestFlagPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		defaults    map[string]string
		checkValues map[string]string
	}{
		{
			name: "flags override defaults",
			args: []string{"-secret", "flag-secret", "-node-id", "flag-node"},
			defaults: map[string]string{
				"secret":  "default-secret",
				"node-id": "default-node",
			},
			checkValues: map[string]string{
				"secret":  "flag-secret",
				"node-id": "flag-node",
			},
		},
		{
			name: "unset flags use defaults",
			args: []string{"-secret", "flag-secret"},
			defaults: map[string]string{
				"secret":  "default-secret",
				"node-id": "default-node",
			},
			checkValues: map[string]string{
				"secret":  "flag-secret",
				"node-id": "default-node",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create flag set
			fs := flag.NewFlagSet("test", flag.ContinueOnError)

			// Variables with defaults
			secret := tt.defaults["secret"]
			nodeID := tt.defaults["node-id"]

			// Define flags
			fs.StringVar(&secret, "secret", secret, "Secret")
			fs.StringVar(&nodeID, "node-id", nodeID, "Node ID")

			// Parse
			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			// Check values
			if secret != tt.checkValues["secret"] {
				t.Errorf("secret = %q, want %q", secret, tt.checkValues["secret"])
			}
			if nodeID != tt.checkValues["node-id"] {
				t.Errorf("node-id = %q, want %q", nodeID, tt.checkValues["node-id"])
			}
		})
	}
}

// Helper function.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Benchmark tests

func BenchmarkStringSliceFlagSet(b *testing.B) {
	benchmarks := []struct {
		name  string
		value string
	}{
		{"single", "host:8080"},
		{"comma-separated-3", "host1:8080,host2:8080,host3:8080"},
		{"comma-separated-10", "h1:8080,h2:8080,h3:8080,h4:8080,h5:8080,h6:8080,h7:8080,h8:8080,h9:8080,h10:8080"},
		{"with-spaces", "  host1:8080  ,  host2:8080  ,  host3:8080  "},
		{"with-empty", "host1:8080,,host2:8080,,,host3:8080"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				var flag stringSliceFlag
				_ = flag.Set(bm.value)
			}
		})
	}
}

func BenchmarkValidateAddress(b *testing.B) {
	addresses := []string{
		"localhost:8080",
		"example.com:8080",
		"192.168.1.1:8080",
		":8080",
		"[::1]:8080",
		"very-long-hostname-with-many-parts.example.com:8080",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, addr := range addresses {
			_ = validateAddress(addr)
		}
	}
}
