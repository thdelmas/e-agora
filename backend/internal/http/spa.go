package http

import (
	"bytes"
	"html"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// seoMeta is the per-route <head> content injected into the SPA shell. The app
// renders client-side, so without this every URL would share one generic
// title/description and crawlers (and link unfurlers) would see nothing
// route-specific. See injectSEO.
type seoMeta struct {
	title       string
	description string
}

// seoPages maps a canonical path to its metadata. The "/" entry MUST match the
// default <title>/<meta description> baked into frontend/index.html
// (seoDefaultTitle / seoDefaultDesc) — the homepage reuses the file's defaults.
var seoPages = map[string]seoMeta{
	"/": {
		title:       "e-agora — the people decide",
		description: "A worldwide ranking of public figures, decided one head-to-head at a time in the digital agora.",
	},
	"/leaderboard": {
		title:       "World rankings — e-agora",
		description: "Live world rankings of public figures, forged head-to-head from the aggregated preferences of anonymous visitors.",
	},
	"/stats": {
		title:       "Transparency stats — e-agora",
		description: "Live aggregate stats for e-agora: votes cast, visitors, and figures added over time — anonymous counts only, nothing traceable to a visitor.",
	},
}

// sitemapPaths lists the public, ungated routes advertised in /sitemap.xml, in a
// fixed order for deterministic output. /leaderboard is intentionally omitted: it
// is gated behind a 24h access token and redirects unauthenticated visitors.
var sitemapPaths = []string{"/", "/stats"}

// Defaults baked into frontend/index.html that the backend swaps per route. Keep
// them byte-identical to the file, or the swap silently no-ops (page still works,
// it just keeps the homepage title/description).
const (
	seoDefaultTitle = "<title>e-agora — the people decide</title>"
	seoDefaultDesc  = `<meta name="description" content="A worldwide ranking of public figures, decided one head-to-head at a time in the digital agora." />`
)

// spaHandler serves the built single-page app from dir (M6, production
// same-origin serving): a real file when one exists, otherwise index.html so
// client-side routes like /leaderboard resolve. It additionally serves a dynamic
// /robots.txt and /sitemap.xml, and injects per-route SEO tags into the index
// fallback. It is mounted on the non-/api catch-all only when EAGORA_STATIC_DIR
// is set. http.Dir constrains paths, so ".." traversal is rejected. publicURL is
// the canonical origin for those tags (EAGORA_PUBLIC_URL; empty → derived from
// the request).
func spaHandler(dir, publicURL string) http.HandlerFunc {
	root := http.Dir(dir)
	fileServer := http.FileServer(root)
	indexPath := filepath.Join(dir, "index.html")
	indexHTML, _ := os.ReadFile(indexPath) // read once; empty → plain ServeFile fallback

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			writeRobots(w, r, publicURL)
			return
		case "/sitemap.xml":
			writeSitemap(w, r, publicURL)
			return
		}

		// Serve a real static asset (JS/CSS/icons/og.png/manifest) when one exists.
		if f, err := root.Open(r.URL.Path); err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback: serve index.html with per-route SEO tags injected so each
		// client-side route is crawlable and unfurls correctly when shared.
		if len(indexHTML) == 0 {
			http.ServeFile(w, r, indexPath) // index missing at read time — best effort
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(injectSEO(indexHTML, r, publicURL))
	}
}

// injectSEO returns index with this route's <title>/description swapped in and a
// block of canonical + Open Graph + Twitter Card tags inserted before </head>.
// Unknown paths (the SPA's soft 404s) are marked noindex and canonicalised to the
// homepage so junk URLs are not indexed.
func injectSEO(index []byte, r *http.Request, publicURL string) []byte {
	canonPath := path.Clean(r.URL.Path)
	if canonPath == "." || canonPath == "" {
		canonPath = "/"
	}
	meta, known := seoPages[canonPath]
	base := baseURL(r, publicURL)

	doc := index
	var head strings.Builder

	if !known {
		head.WriteString(`<meta name="robots" content="noindex" />` + "\n    ")
		meta = seoPages["/"]
		canonPath = "/"
	} else if canonPath != "/" {
		doc = bytes.Replace(doc, []byte(seoDefaultTitle),
			[]byte("<title>"+html.EscapeString(meta.title)+"</title>"), 1)
		doc = bytes.Replace(doc, []byte(seoDefaultDesc),
			[]byte(`<meta name="description" content="`+html.EscapeString(meta.description)+`" />`), 1)
	}

	canonical := html.EscapeString(base + canonPath)
	title := html.EscapeString(meta.title)
	desc := html.EscapeString(meta.description)
	image := html.EscapeString(base + "/og.png")

	head.WriteString(`<link rel="canonical" href="` + canonical + `" />` + "\n    ")
	head.WriteString(`<meta property="og:type" content="website" />` + "\n    ")
	head.WriteString(`<meta property="og:site_name" content="e-agora" />` + "\n    ")
	head.WriteString(`<meta property="og:title" content="` + title + `" />` + "\n    ")
	head.WriteString(`<meta property="og:description" content="` + desc + `" />` + "\n    ")
	head.WriteString(`<meta property="og:url" content="` + canonical + `" />` + "\n    ")
	head.WriteString(`<meta property="og:image" content="` + image + `" />` + "\n    ")
	head.WriteString(`<meta name="twitter:card" content="summary_large_image" />` + "\n    ")
	head.WriteString(`<meta name="twitter:title" content="` + title + `" />` + "\n    ")
	head.WriteString(`<meta name="twitter:description" content="` + desc + `" />` + "\n    ")
	head.WriteString(`<meta name="twitter:image" content="` + image + `" />` + "\n  ")

	return bytes.Replace(doc, []byte("</head>"), []byte(head.String()+"</head>"), 1)
}

// baseURL returns the absolute origin (scheme://host, no trailing slash) for
// canonical/OG URLs and the sitemap. EAGORA_PUBLIC_URL is authoritative when set
// — a single fixed host avoids duplicate-content signals across the *.onrender.com
// URL and any custom domain. Otherwise it is derived from the request, honoring
// the proxy headers Render terminates TLS with.
func baseURL(r *http.Request, publicURL string) string {
	if publicURL != "" {
		return publicURL
	}
	scheme := "http"
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return scheme + "://" + host
}

func writeRobots(w http.ResponseWriter, r *http.Request, publicURL string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "User-agent: *\nAllow: /\nDisallow: /api/\n\nSitemap: "+baseURL(r, publicURL)+"/sitemap.xml\n")
}

func writeSitemap(w http.ResponseWriter, r *http.Request, publicURL string) {
	base := baseURL(r, publicURL)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range sitemapPaths {
		b.WriteString("  <url><loc>" + html.EscapeString(base+p) + "</loc><changefreq>daily</changefreq></url>\n")
	}
	b.WriteString("</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	io.WriteString(w, b.String())
}
