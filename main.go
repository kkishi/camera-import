package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type file struct {
	path    string
	isMedia bool
	size    int64
	time    time.Time
}

func exifCreateDate(path string) time.Time {
	cmd := exec.Command("exiftool", "-CreateDate", "-s", "-S", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("%v\nStdout: %q\nStderr: %q", err, stdout.String(), stderr.String())
	}
	timeStr := strings.TrimSpace(stdout.String())
	if t, err := time.ParseInLocation("2006:01:02 15:04:05", timeStr, time.Local); err == nil {
		return t
	}
	if t, err := time.Parse("2006:01:02 15:04:05-07:00", timeStr); err == nil {
		return t
	}
	log.Fatalf("unrecognized timeStr: %q", timeStr)
	return time.Time{}
}

type walkDirFuncArgs struct {
	path string
	d    fs.DirEntry
}

func walkDir(dir string) chan *walkDirFuncArgs {
	ch := make(chan *walkDirFuncArgs)
	go func() {
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			ch <- &walkDirFuncArgs{path: path, d: d}
			return err
		}); err != nil {
			log.Fatal(err)
		}
		close(ch)
	}()
	return ch
}

func readFiles(in chan *walkDirFuncArgs) chan *file {
	out := make(chan *file)
	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			for args := range in {
				if args.d.IsDir() {
					continue
				}
				i, err := args.d.Info()
				if err != nil {
					log.Fatal(err)
				}
				isMedia := false
				var t time.Time
				switch strings.ToLower(filepath.Ext(args.path)) {
				case
					".cr2",
					".cr3",
					".jpg",
					".mov",
					".mp4",
					".rw2":
					isMedia = true
					t = exifCreateDate(args.path)
				}
				out <- &file{
					path:    args.path,
					isMedia: isMedia,
					size:    i.Size(),
					time:    t,
				}
			}
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func listFiles(dir string) []*file {
	var files []*file
	for f := range readFiles(walkDir(dir)) {
		icon := "🗒"
		if f.isMedia {
			icon = "🖼"
		}
		fmt.Printf("%s %s %d %v\n", icon, f.path, f.size, f.time)
		files = append(files, f)
	}
	return files
}

var (
	src    = flag.String("src", "", "")
	dst    = flag.String("dst", "", "")
	camera = flag.String("camera", "", "")
)

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	switch *camera {
	case "5dii":
		*src = "/media/keisuke/EOS_DIGITAL"
		*dst = "/tank/photos/keisuke/Pictures/5DII"
	case "gh5":
		*src = "/media/keisuke/LUMIX"
		*dst = "/tank/photos/keisuke/Pictures/GH5"
	case "r6":
		*src = "/media/keisuke/EOS_DIGITAL"
		*dst = "/tank/photos/keisuke/Pictures/R6"
	case "gopro":
		*src = "/media/keisuke/7000-8000"
		*dst = "/tank/photos/keisuke/Pictures/GOPRO"
	case "gm1":
		*dst = "/tank/photos/keisuke/Pictures/GM1"
	case "gx1s":
		*dst = "/tank/photos/keisuke/Pictures/GX1S"
	case "gx1b":
		*dst = "/tank/photos/keisuke/Pictures/GX1B"
	}
	if *src == "" || *dst == "" {
		log.Fatal("invalid arguments")
	}

	metadata := make(map[string]int)
	var totalBytes, metadataBytes int64
	var lo, hi time.Time
	for _, f := range listFiles(*src) {
		totalBytes += f.size
		if !f.isMedia {
			metadata[filepath.Ext(f.path)]++
			metadataBytes += f.size
			continue
		}
		if lo.IsZero() || lo.After(f.time) {
			lo = f.time
		}
		if hi.IsZero() || hi.Before(f.time) {
			hi = f.time
		}
	}
	fmt.Printf("metadata: %d/%d (%f%%)\n", metadataBytes, totalBytes, float64(metadataBytes)/float64(totalBytes)*100)
	fmt.Println(metadata)

	cmd := exec.Command("rsync", "-Pav", filepath.Clean(*src)+"/", filepath.Join(*dst, lo.Format("20060102")+"_"+hi.Format("20060102"))+"/")
	fmt.Printf("Running the command (y/n): %s\n> ", cmd.String())
	var input string
	fmt.Scanf("%s", input)
	if input != "y" {
		return
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}
