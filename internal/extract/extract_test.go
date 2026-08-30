package extract

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		pageURL  string
		html     string
		wantLink []string // Resolved URLs, in order
		wantSkip int
	}{
		{
			name:     "resolves relative against directory base",
			pageURL:  "https://ex.com/docs/",
			html:     `<a href="intro">i</a>`,
			wantLink: []string{"https://ex.com/docs/intro"},
		},
		{
			name:     "relative replaces last segment when base is a file",
			pageURL:  "https://ex.com/docs",
			html:     `<a href="intro">i</a>`,
			wantLink: []string{"https://ex.com/intro"},
		},
		{
			name:     "root-relative ignores path",
			pageURL:  "https://ex.com/a/b/c",
			html:     `<a href="/about">a</a>`,
			wantLink: []string{"https://ex.com/about"},
		},
		{
			name:     "base href overrides page URL",
			pageURL:  "https://ex.com/page",
			html:     `<base href="https://other.com/x/"><a href="y">y</a>`,
			wantLink: []string{"https://other.com/x/y"},
		},
		{
			name:     "skips mailto and tel",
			pageURL:  "https://ex.com/",
			html:     `<a href="mailto:a@b.com">m</a><a href="tel:+123">t</a>`,
			wantSkip: 2,
		},
		{
			name:     "keeps fragment separately",
			pageURL:  "https://ex.com/",
			html:     `<a href="/docs#install">d</a>`,
			wantLink: []string{"https://ex.com/docs#install"},
		},
		{
			name:     "absolute external link is kept",
			pageURL:  "https://ex.com/",
			html:     `<a href="https://golang.org/doc">g</a>`,
			wantLink: []string{"https://golang.org/doc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.pageURL, []byte(tt.html))
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if len(got.Links) != len(tt.wantLink) {
				t.Fatalf("got %d links, want %d: %+v", len(got.Links), len(tt.wantLink), got.Links)
			}
			for i, want := range tt.wantLink {
				if got.Links[i].Resolved != want {
					t.Errorf("link %d = %q, want %q", i, got.Links[i].Resolved, want)
				}
			}
			if len(got.Skipped) != tt.wantSkip {
				t.Errorf("got %d skipped, want %d: %+v", len(got.Skipped), tt.wantSkip, got.Skipped)
			}
		})
	}
}