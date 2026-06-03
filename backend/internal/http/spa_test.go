package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestSPA lays down a minimal built-SPA dir: an index.html carrying the
// same default <title>/<meta description> as frontend/index.html, plus one
// real asset.
func writeTestSPA(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	index := `<!doctype html><html lang="en"><head>` +
		seoDefaultTitle +
		seoDefaultDesc +
		`</head><body><div id="app"></div></body></html>`
	if err := os.WriteFile(
		filepath.Join(dir, "index.html"), []byte(index), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	return dir
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr.Code, rr.Body.String()
}

func TestSPA_InjectsPerRouteSEO(t *testing.T) {
	h := spaHandler(writeTestSPA(t), "https://e-agora.test")

	// Homepage keeps the file's default title; canonical/OG resolve to the root.
	_, home := get(t, h, "/")
	for _, want := range []string{
		seoDefaultTitle,
		`<link rel="canonical" href="https://e-agora.test/" />`,
		`<meta property="og:title" content="e-agora — the people decide" />`,
		`<meta property="og:image" content="https://e-agora.test/og.png" />`,
		`<meta name="twitter:card" content="summary_large_image" />`,
	} {
		if !strings.Contains(home, want) {
			t.Errorf("homepage missing %q", want)
		}
	}
	if strings.Contains(home, "noindex") {
		t.Error("homepage should be indexable")
	}

	// A known sub-route swaps the title/description and canonicalises to itself.
	_, stats := get(t, h, "/stats")
	if !strings.Contains(stats, "<title>Transparency stats — e-agora</title>") {
		t.Error("/stats title not swapped in")
	}
	if strings.Contains(stats, seoDefaultTitle) {
		t.Error("/stats still carries the default title")
	}
	if !strings.Contains(stats,
		`<link rel="canonical" href="https://e-agora.test/stats" />`) {
		t.Error("/stats canonical wrong")
	}

	// Unknown route → noindex, canonical pinned to the homepage
	// (soft-404 guard).
	_, unknown := get(t, h, "/does-not-exist")
	if !strings.Contains(unknown, `<meta name="robots" content="noindex" />`) {
		t.Error("unknown route should be noindex")
	}
	if !strings.Contains(unknown,
		`<link rel="canonical" href="https://e-agora.test/" />`) {
		t.Error("unknown route canonical should point home")
	}
}

func TestSPA_RealAssetNotInjected(t *testing.T) {
	h := spaHandler(writeTestSPA(t), "https://e-agora.test")
	code, body := get(t, h, "/app.js")
	if code != http.StatusOK || body != "console.log(1)" {
		t.Errorf("asset should be served verbatim, got %d %q", code, body)
	}
}

func TestSPA_RobotsAndSitemap(t *testing.T) {
	h := spaHandler(writeTestSPA(t), "https://e-agora.test")

	code, robots := get(t, h, "/robots.txt")
	if code != http.StatusOK {
		t.Fatalf("robots status %d", code)
	}
	for _, want := range []string{
		"Disallow: /api/", "Sitemap: https://e-agora.test/sitemap.xml",
	} {
		if !strings.Contains(robots, want) {
			t.Errorf("robots.txt missing %q", want)
		}
	}

	code, sitemap := get(t, h, "/sitemap.xml")
	if code != http.StatusOK {
		t.Fatalf("sitemap status %d", code)
	}
	for _, want := range []string{
		"<loc>https://e-agora.test/</loc>",
		"<loc>https://e-agora.test/stats</loc>",
	} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap missing %q", want)
		}
	}
	// The gated leaderboard must not be advertised.
	if strings.Contains(sitemap, "/leaderboard") {
		t.Error("sitemap should not list the gated /leaderboard")
	}
}

// baseURL falls back to the request origin (honoring proxy headers) when no
// EAGORA_PUBLIC_URL is configured.
func TestBaseURL_FallbackFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "app.onrender.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := baseURL(req, ""); got != "https://app.onrender.com" {
		t.Errorf("baseURL fallback = %q", got)
	}
	if got := baseURL(req, "https://fixed.example"); got !=
		"https://fixed.example" {
		t.Errorf("explicit publicURL should win, got %q", got)
	}
}
