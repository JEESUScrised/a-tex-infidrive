package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolve подбирает пути к ffmpeg и ffprobe.
// Порядок: явный аргумент → FFMPEG_PATH/FFPROBE_PATH → YTDRIVE_FFMPEG/YTDRIVE_FFPROBE →
// локальный каталог .tools/*/bin рядом с рабочей директорией → имена из PATH.
// Поиск по всему диску не выполняется.
func Resolve(ffmpegHint, ffprobeHint string) (ffmpegPath, ffprobePath, source string) {
	ff, ffSrc := pickBinary(ffmpegHint, "ffmpeg", "FFMPEG_PATH", "YTDRIVE_FFMPEG")
	fp, fpSrc := pickBinary(ffprobeHint, "ffprobe", "FFPROBE_PATH", "YTDRIVE_FFPROBE")
	src := mergeSource(ffSrc, fpSrc)
	return ff, fp, src
}

func mergeSource(a, b string) string {
	rank := func(s string) int {
		switch {
		case s == "flag":
			return 4
		case s == "env":
			return 3
		case s == "local:.tools":
			return 2
		case s == "PATH":
			return 1
		default:
			return 0
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	return a
}

// Diagnose сообщает, найден ли FFmpeg и откуда.
func Diagnose(ffmpegHint, ffprobeHint string) string {
	ff, fp, src := Resolve(ffmpegHint, ffprobeHint)
	r := New(ff, fp)
	path, err := r.look(r.FFmpeg)
	if err != nil {
		return fmt.Sprintf("ffmpeg: не найден (источник %s, значение %q)", src, ff)
	}
	probePath, probeErr := r.look(r.FFProbe)
	probeInfo := probePath
	if probeErr != nil {
		probeInfo = fmt.Sprintf("не найден (%q)", fp)
	}
	return fmt.Sprintf("ffmpeg: %s; ffprobe: %s; источник: %s", path, probeInfo, src)
}

func pickBinary(hint, defaultName string, envKeys ...string) (string, string) {
	hint = strings.TrimSpace(hint)
	if hint != "" && !isDefaultName(hint, defaultName) {
		if resolved := resolveOne(hint); resolved != "" {
			return resolved, "flag"
		}
		if looksLikePath(hint) {
			return hint, "flag"
		}
	}
	for _, key := range envKeys {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			continue
		}
		if resolved := resolveOne(v); resolved != "" {
			return resolved, "env"
		}
		if looksLikePath(v) {
			return v, "env"
		}
	}
	if localFF, localFP, ok := findLocalTools(); ok {
		if defaultName == "ffmpeg" || strings.HasPrefix(defaultName, "ffmpeg") {
			return localFF, "local:.tools"
		}
		return localFP, "local:.tools"
	}
	if p, err := exec.LookPath(defaultName); err == nil {
		return p, "PATH"
	}
	return defaultName, "default"
}

func isDefaultName(name, defaultName string) bool {
	base := strings.ToLower(filepath.Base(name))
	def := strings.ToLower(defaultName)
	return base == def || base == def+".exe"
}

func looksLikePath(name string) bool {
	return filepath.IsAbs(name) || strings.ContainsAny(name, `/\`)
}

func resolveOne(name string) string {
	if looksLikePath(name) {
		if st, err := os.Stat(name); err == nil && !st.IsDir() {
			return name
		}
		return ""
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func findLocalTools() (ffmpegPath, ffprobePath string, ok bool) {
	exeName := "ffmpeg"
	probeName := "ffprobe"
	if runtime.GOOS == "windows" {
		exeName = "ffmpeg.exe"
		probeName = "ffprobe.exe"
	}
	roots := []string{"."}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd)
	}
	seen := map[string]struct{}{}
	for _, root := range roots {
		root = filepath.Clean(root)
		if _, dup := seen[root]; dup {
			continue
		}
		seen[root] = struct{}{}
		pattern := filepath.Join(root, ".tools", "*", "bin", exeName)
		matches, _ := filepath.Glob(pattern)
		for _, ff := range matches {
			dir := filepath.Dir(ff)
			fp := filepath.Join(dir, probeName)
			if _, err := os.Stat(fp); err != nil {
				continue
			}
			if abs, err := filepath.Abs(ff); err == nil {
				ff = abs
			}
			if abs, err := filepath.Abs(fp); err == nil {
				fp = abs
			}
			return ff, fp, true
		}
	}
	return "", "", false
}
