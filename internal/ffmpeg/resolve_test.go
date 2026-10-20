package ffmpeg

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveFindsLocalTools(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, ".tools", "demo-build", "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	exe := "ffmpeg"
	probe := "ffprobe"
	if runtime.GOOS == "windows" {
		exe += ".exe"
		probe += ".exe"
	}
	ffPath := filepath.Join(bin, exe)
	fpPath := filepath.Join(bin, probe)
	if err := os.WriteFile(ffPath, []byte("x"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fpPath, []byte("x"), 0o750); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	ff, fp, src := Resolve("", "")
	if src != "local:.tools" {
		t.Fatalf("source %s", src)
	}
	if ff != ffPath || fp != fpPath {
		t.Fatalf("got %s %s", ff, fp)
	}
}

func TestResolveKeepsMissingExplicitPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-ffmpeg")
	ff, _, src := Resolve(missing, "")
	if ff != missing {
		t.Fatalf("path replaced: %s", ff)
	}
	if src != "flag" {
		t.Fatalf("source %s", src)
	}
}

func TestDiagnoseMentionsSource(t *testing.T) {
	msg := Diagnose("ytdrive-ffmpeg-missing-xyz", "ytdrive-ffprobe-missing-xyz")
	if msg == "" {
		t.Fatal("empty diagnose")
	}
}
