package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

var sizes = []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}

func FormatFileSize(size uint64) string {
	unitsLimit := len(sizes)
	i := 0
	fs := float64(size)
	for fs >= 1024.00 && i < unitsLimit {
		fs = fs / 1024.00
		i++
	}

	f := "%.0f %s"
	if i > 1 {
		f = "%.2f %s"
	}

	return fmt.Sprintf(f, fs, sizes[i])
}

func Timestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

func Now() time.Time {
	return time.Now()
}

func TimeTrack(start time.Time, name string) {
	// usage: defer utils.TimeTrack(utils.Now(), "tag")   -- on the begin of any func
	elapsed := time.Since(start)
	fmt.Printf("%s took %s\n", name, elapsed)
}

type FileInfo struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

func ReadDir(rootPath string, recursive bool) (<-chan FileInfo, error) {
	var err error
	channel := make(chan FileInfo)

	var readDirRec func(dirPath string)
	readDirRec = func(dirPath string) {
		entries, _err := os.ReadDir(dirPath)
		if _err != nil {
			err = _err
			return
		}
		for i := range entries {
			var fi FileInfo
			info, _err := entries[i].Info()
			if err == nil {
				fi.Name = info.Name()
				fi.Size = info.Size()
				fi.IsDir = info.IsDir()
				fi.ModTime = info.ModTime()
			} else {
				err = _err
				return
			}
			if recursive && fi.IsDir {
				readDirRec(path.Join(dirPath, fi.Name))
			} else {
				channel <- fi
			}
		}
	}

	go func() {
		readDirRec(rootPath)
		close(channel)
	}()
	return channel, err
}

func LogError(err error) {
	pc, file, line, _ := runtime.Caller(1)
	_, fileName := path.Split(file)
	parts := strings.Split(runtime.FuncForPC(pc).Name(), ".")
	funcName := strings.Join(parts[1:], ".")
	log.Printf("[%s/%s(%s)] %s: %s", parts[0][11:], fileName, strconv.Itoa(line), funcName, err)
}

func WithTimeout(cbfn func() error, ms time.Duration) error {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), ms*time.Millisecond)
	defer cancel()
	done := make(chan struct{}, 1)

	go func() {
		err = cbfn()
		done <- struct{}{}
		close(done)
	}()

	select {
	case <-done:
		return err
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "operation timeout")
	}

}
