package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		`..\..\windows\system32\evil.txt`: "evil.txt",
		`../../etc/passwd`:                "passwd",
		`good.zip`:                        "good.zip",
		``:                                "file.bin",
		`con.txt`:                         "con.txt",
		`a/b\c?.bin`:                      "c_.bin",
	}
	for in, want := range cases {
		if got := SanitizeName(in); got != want {
			t.Fatalf("%q -> %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("a", 300) + ".zip"
	got := SanitizeName(long)
	if len(got) > 180 {
		t.Fatalf("len %d", len(got))
	}
	if !strings.HasSuffix(got, ".zip") {
		t.Fatalf("%s", got)
	}
}

func TestSaveAndJobs(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	f, err := st.SaveUpload(bytes.NewReader([]byte("hello")), `..\secret.txt`, "text/plain", 100)
	if err != nil {
		t.Fatal(err)
	}
	if f.OriginalName != "secret.txt" {
		t.Fatalf("name %s", f.OriginalName)
	}
	if _, err := os.Stat(f.StoredPath); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.StoredPath, filepath.Join("objects", f.ID)) {
		t.Fatalf("path %s", f.StoredPath)
	}
	_, err = st.SaveUpload(bytes.NewReader([]byte("too-big")), "a.bin", "", 3)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
	list, err := st.ListFiles()
	if err != nil || len(list) != 1 {
		t.Fatalf("list %d %v", len(list), err)
	}
	job, err := st.CreateJob(KindEncode, f.ID, f.StoredPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateJob(job.ID, StatusSucceeded, 100, "готово", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetJob("../" + job.ID); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("traversal id: %v", err)
	}
	got, err := st.GetJob(job.ID)
	if err != nil || got.Status != StatusSucceeded {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestReopenPreservesFiles(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := st.SaveUpload(bytes.NewReader([]byte("persist")), "keep.txt", "text/plain", 100)
	if err != nil {
		t.Fatal(err)
	}
	id := f.ID
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	got, err := st2.GetFile(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginalName != "keep.txt" || got.Size != 7 {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(got.StoredPath); err != nil {
		t.Fatal(err)
	}
}
