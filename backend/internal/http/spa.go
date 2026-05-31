package http

import (
	"net/http"
	"path/filepath"
)

// spaHandler serves the built single-page app from dir (M6, production
// same-origin serving): a real file when one exists, otherwise index.html so
// client-side routes like /leaderboard resolve. It is mounted on the non-/api
// catch-all only when EAGORA_STATIC_DIR is set. http.Dir constrains paths, so
// ".." traversal is rejected.
func spaHandler(dir string) http.HandlerFunc {
	root := http.Dir(dir)
	fileServer := http.FileServer(root)
	index := filepath.Join(dir, "index.html")

	return func(w http.ResponseWriter, r *http.Request) {
		if f, err := root.Open(r.URL.Path); err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFile(w, r, index) // SPA fallback
	}
}
