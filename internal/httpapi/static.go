package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaFileServer отдаёт файлы из dist и index.html для клиентских маршрутов React Router.
func spaFileServer(root string) http.Handler {
	root = filepath.Clean(root)
	fs := http.FileServer(http.Dir(root))
	index := filepath.Join(root, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		reqPath := filepath.Clean("/" + r.URL.Path)
		if reqPath == "/" || reqPath == "." {
			http.ServeFile(w, r, index)
			return
		}
		full := filepath.Join(root, strings.TrimPrefix(reqPath, "/"))
		rel, err := filepath.Rel(root, full)
		if err != nil || strings.HasPrefix(rel, "..") {
			http.NotFound(w, r)
			return
		}
		info, err := os.Stat(full)
		if err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}

func resolveWebDir(configured string) string {
	if configured != "" {
		return configured
	}
	candidates := []string{
		filepath.Join("frontend", "dist"),
		"web",
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "index.html")); err == nil {
			return c
		}
	}
	return filepath.Join("frontend", "dist")
}
