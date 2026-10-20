package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"ytdrive/internal/codec"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "encode":
		os.Exit(cmdEncode(os.Args[2:]))
	case "decode":
		os.Exit(cmdDecode(os.Args[2:]))
	case "verify":
		os.Exit(cmdVerify(os.Args[2:]))
	case "-h", "--help", "help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `YTDrive — кодирование файла в MP4 и восстановление обратно.

Команды:
  ytdrive encode --input file.zip --output video.mp4
  ytdrive decode --input video.mp4 --output restored.zip
  ytdrive verify --original file.zip --restored restored.zip

Флаги encode: --fps --width --height --block --crf --preset --bitrate
  --gop --frame-repeat --packet-repeat --parity-group --chunk --gzip
  --ffmpeg --ffprobe
Флаги decode: --width --height --block --ffmpeg --ffprobe
  (размеры 0 означают автоопределение сетки)
`)
}

func cmdEncode(args []string) int {
	opt := codec.DefaultOptions()
	fs := flag.NewFlagSet("encode", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	input := fs.String("input", "", "исходный файл")
	output := fs.String("output", "", "выходной MP4")
	fs.IntVar(&opt.FPS, "fps", opt.FPS, "кадров в секунду")
	fs.IntVar(&opt.Width, "width", opt.Width, "ширина кадра")
	fs.IntVar(&opt.Height, "height", opt.Height, "высота кадра")
	fs.IntVar(&opt.Block, "block", opt.Block, "сторона блока в пикселях")
	fs.IntVar(&opt.CRF, "crf", opt.CRF, "качество libx264 (меньше — крупнее файл)")
	fs.StringVar(&opt.Preset, "preset", opt.Preset, "preset libx264")
	fs.IntVar(&opt.BitrateK, "bitrate", 0, "битрейт кбит/с, 0 = использовать crf")
	fs.IntVar(&opt.GOP, "gop", opt.GOP, "интервал ключевых кадров")
	fs.IntVar(&opt.FrameRepeat, "frame-repeat", opt.FrameRepeat, "сколько раз записать каждый кадр")
	fs.IntVar(&opt.PacketRepeat, "packet-repeat", opt.PacketRepeat, "сколько копий каждого пакета")
	fs.IntVar(&opt.ParityGroup, "parity-group", opt.ParityGroup, "размер XOR-группы, 0 = выключить")
	fs.IntVar(&opt.ChunkSize, "chunk", opt.ChunkSize, "байт полезных данных в пакете")
	fs.BoolVar(&opt.Gzip, "gzip", false, "сжать данные перед укладкой в кадры")
	fs.StringVar(&opt.FFmpeg, "ffmpeg", opt.FFmpeg, "путь к ffmpeg")
	fs.StringVar(&opt.FFProbe, "ffprobe", opt.FFProbe, "путь к ffprobe")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *input == "" || *output == "" {
		fs.Usage()
		return 2
	}
	start := time.Now()
	last := -1
	res, err := codec.EncodeFile(context.Background(), *input, *output, opt, func(done, total int) {
		pct := 0
		if total > 0 {
			pct = done * 100 / total
		}
		if pct/10 == last {
			return
		}
		last = pct / 10
		fmt.Fprintf(os.Stderr, "кодирование %d%%\n", pct)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %v\n", err)
		return 1
	}
	fmt.Printf("готово: %s\n", *output)
	fmt.Printf("исходный размер: %d байт\n", res.OriginalSize)
	fmt.Printf("в кадрах: %d байт\n", res.StoredSize)
	fmt.Printf("sha256: %s\n", res.SHA256Hex)
	fmt.Printf("кадров: %d, пакетов: %d, %dx%d\n", res.Frames, res.Packets, res.Width, res.Height)
	fmt.Printf("время: %s\n", time.Since(start).Round(time.Millisecond))
	return 0
}

func cmdDecode(args []string) int {
	fs := flag.NewFlagSet("decode", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	input := fs.String("input", "", "видео MP4")
	output := fs.String("output", "", "куда записать восстановленный файл")
	var opt codec.Options
	fs.IntVar(&opt.Width, "width", 0, "логическая ширина, 0 = авто")
	fs.IntVar(&opt.Height, "height", 0, "логическая высота, 0 = авто")
	fs.IntVar(&opt.Block, "block", 0, "размер блока, 0 = авто")
	fs.StringVar(&opt.FFmpeg, "ffmpeg", "ffmpeg", "путь к ffmpeg")
	fs.StringVar(&opt.FFProbe, "ffprobe", "ffprobe", "путь к ffprobe")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *input == "" || *output == "" {
		fs.Usage()
		return 2
	}
	res, err := codec.DecodeFile(context.Background(), *input, *output, opt, func(done, total int) {
		if total > 0 && (done == total || done%5 == 0) {
			fmt.Fprintf(os.Stderr, "чтение кадров %d/%d\n", done, total)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %v\n", err)
		return 1
	}
	fmt.Printf("восстановлен: %s\n", *output)
	fmt.Printf("имя из видео: %s (%s)\n", res.Name, res.MIME)
	fmt.Printf("размер: %d байт\n", res.Size)
	fmt.Printf("sha256: %s\n", res.SHA256Hex)
	fmt.Printf("кадры: принято %d, уникальных %d, дубликатов %d, пропущено %d\n",
		res.FramesSeen, res.UniqueFrames, res.DuplicateFrames, res.SkippedFrames)
	fmt.Printf("пакеты: повреждено %d, восстановлено XOR %d\n", res.CorruptPackets, res.RecoveredPackets)
	return 0
}

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	original := fs.String("original", "", "исходный файл")
	restored := fs.String("restored", "", "восстановленный файл")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *original == "" || *restored == "" {
		fs.Usage()
		return 2
	}
	sum, err := codec.Verify(*original, *restored)
	if err != nil {
		fmt.Fprintf(os.Stderr, "не совпадает: %v\n", err)
		return 1
	}
	fmt.Printf("OK %s\n", sum)
	return 0
}
