package utils

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"torrentino/common/logging"
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
	logging.Info(logging.TagNone, "%s took %s", name, elapsed)
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
			if _err != nil {
				err = _err
				return
			}
			fi.Name = info.Name()
			fi.Size = info.Size()
			fi.IsDir = info.IsDir()
			fi.ModTime = info.ModTime()
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
	pkg := strings.TrimPrefix(parts[0], "torrentino/")
	logging.Error(logTag(pkg), "%s/%s:%d %s: %s", pkg, fileName, line, funcName, err)
}

// logTag maps the package that called LogError to the interface tag. Shared
// packages (core, common/list, common/settings) have no tag: the same code is
// reached from both the web UI and the Telegram bot.
func logTag(pkg string) logging.Tag {
	switch {
	case pkg == "web" || strings.HasPrefix(pkg, "web/"):
		return logging.TagWeb
	case pkg == "telegram" || strings.HasPrefix(pkg, "telegram/"),
		pkg == "common/paginator" || strings.HasPrefix(pkg, "common/paginator/"):
		return logging.TagTGBot
	default:
		return logging.TagNone
	}
}

func WithTimeout(cbfn func() error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	done := make(chan error, 1)

	go func() {
		done <- cbfn()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "operation timeout")
	}

}
