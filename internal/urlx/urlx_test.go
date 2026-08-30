package urlx

import "testing"

func TestIdentity(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases host", "https://Example.COM/about", "https://example.com/about"},
		{"preserves path case", "https://example.com/About", "https://example.com/About"},
		{"strips default https port", "https://example.com:443/about", "https://example.com/about"},
		{"strips default http port", "http://example.com:80/about", "http://example.com/about"},
		{"keeps non-default port", "https://example.com:8080/about", "https://example.com:8080/about"},
		{"strips trailing slash", "https://example.com/about/", "https://example.com/about"},
		{"keeps root slash", "https://example.com/", "https://example.com/"},
		{"strips fragment", "https://example.com/docs#install", "https://example.com/docs"},
		{"keeps query", "https://example.com/p?id=1", "https://example.com/p?id=1"},
		{"empty path becomes root", "https://example.com", "https://example.com/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Identity(tt.in)
			if err != nil {
				t.Fatalf("Identity(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Identity(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}