package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSPAServesIndexForClientRoutes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>YTDrive</title>"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0o640); err != nil {
		t.Fatal(err)
	}
	h := spaFileServer(dir)

	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/jobs", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "YTDrive") {
		t.Fatalf("body %s", res.Body.String())
	}

	res2 := httptest.NewRecorder()
	h.ServeHTTP(res2, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if res2.Code != http.StatusOK || !strings.Contains(res2.Body.String(), "console.log") {
		t.Fatalf("asset %d %s", res2.Code, res2.Body.String())
	}
}
