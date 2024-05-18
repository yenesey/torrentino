package utils

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"time"
)

var sizes = []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}

func FormatFileSize(size int64, base int) string {
	unitsLimit := len(sizes)
	i := 0
	fs := float64(size)
	fbase := float64(base)
	for fs >= fbase && i < unitsLimit {
		fs = fs / fbase
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
