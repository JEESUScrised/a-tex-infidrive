// Command studiopoc: decode Studio MP4 + decrypt chunk + compare SHA (local PoC).
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"ytdrive/internal/codec"
	"ytdrive/internal/contentcrypto"
	"ytdrive/internal/ffmpeg"
)

func main() {
	in := flag.String("in", "", "studio mp4")
	out := flag.String("out", "", "decoded payload path")
	fileID := flag.String("file-id", "", "drive file id for AAD")
	chunkIndex := flag.Int("chunk-index", 0, "chunk index")
	chunkTotal := flag.Int("chunk-total", 1, "chunk total")
	plainLen := flag.Int64("plain-len", 0, "original chunk plaintext length")
	wantSHA := flag.String("want-sha", "", "expected plaintext sha256 hex")
	keyHex := flag.String("key-hex", "", "CONTENT_ENCRYPTION_KEY hex (32 bytes)")
	flag.Parse()
	if *in == "" || *out == "" || *fileID == "" || *plainLen <= 0 || *keyHex == "" {
		fmt.Fprintln(os.Stderr, "usage: studiopoc -in mp4 -out bin -file-id ID -plain-len N -key-hex HEX [-want-sha HEX]")
		os.Exit(2)
	}
	key, err := hex.DecodeString(*keyHex)
	if err != nil || len(key) != 32 {
		fmt.Fprintln(os.Stderr, "key-hex must be 64 hex chars")
		os.Exit(2)
	}
	ff, fp, _ := ffmpeg.Resolve("", "")
	opt := codec.Options{FFmpeg: ff, FFProbe: fp}
	res, err := codec.DecodeFile(context.Background(), *in, *out, opt, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DECODE_FAIL: %v\n", err)
		os.Exit(3)
	}
	payload, err := os.ReadFile(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
	box, err := contentcrypto.NewBox(key)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(5)
	}
	aad := contentcrypto.ChunkAAD(*fileID, *chunkIndex, *chunkTotal, *plainLen)
	plain, err := box.OpenAAD(payload, aad)
	if err != nil {
		plain, err = box.Open(payload)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "DECRYPT_FAIL: %v\n", err)
		os.Exit(6)
	}
	sum := sha256.Sum256(plain)
	got := hex.EncodeToString(sum[:])
	fmt.Printf("decode_ok name=%q frames=%d payload=%d plain=%d sha=%s\n", res.Name, res.UniqueFrames, len(payload), len(plain), got)
	if *wantSHA != "" {
		fmt.Printf("sha_match=%v\n", got == *wantSHA)
		if got != *wantSHA {
			os.Exit(7)
		}
	}
}
