package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	filesys "github.com/thlib/go-filesys"
)

// FileResult is a data transfer struct for information about a file
type FileResult struct {
	Path  string
	Error error
	Info  os.FileInfo
}

// Checkcopy ...
func Checkcopy(src, dst string) error {
	srcCheck, err := filesys.Checksum(src, md5.New())
	if err != nil {
		return fmt.Errorf("checkcopy src %v %w", src, err)
	}

	dstCheck, err := filesys.Checksum(dst, md5.New())
	if err != nil {
		return fmt.Errorf("checkcopy dst %v %w", dst, err)
	}

	if srcCheck != dstCheck {
		return fmt.Errorf("Source and destination don't match checksum")
	}
	return nil
}

// WalkFilesRecursively goes over all files recursively and returns the name into the channel
func WalkFilesRecursively(root string, c chan FileResult, n int) {
	root, err := filesys.Abs(root)
	if err != nil {
		c <- FileResult{
			Path:  root,
			Error: fmt.Errorf("Failed getting absulute path of %v: %w", root, err),
			Info:  nil,
		}
		return
	}

	file, err := os.Open(root)
	if err != nil {
		close(c)
		c <- FileResult{
			Path:  root,
			Error: fmt.Errorf("failed to read root path: %w", err),
			Info:  nil,
		}
		return
	}
	defer file.Close()
	for {
		names, err := file.Readdirnames(n)
		if err == io.EOF {
			close(c)
			return
		}
		if err != nil {
			close(c)
			c <- FileResult{
				Path:  root,
				Error: fmt.Errorf("failed to read root path: %w", err),
				Info:  nil,
			}
			return
		}
		for _, name := range names {

			path := strings.ReplaceAll(fmt.Sprintf("%v/%v", root, name), "\\", "/")

			fileStat, err := os.Stat(path)
			if os.IsNotExist(err) {
				c <- FileResult{
					Path:  path,
					Error: fmt.Errorf("file does not exist: %w", err),
					Info:  fileStat,
				}
				continue
			}
			if err != nil {
				c <- FileResult{
					Path:  path,
					Error: fmt.Errorf("failed to read file: %w", err),
					Info:  fileStat,
				}
				continue
			}

			if fileStat.IsDir() {
				c2 := make(chan FileResult)
				go WalkFilesRecursively(path, c2, n)
				for file := range c2 {
					c <- file
				}
				continue
			}

			c <- FileResult{
				Path:  path,
				Error: nil,
				Info:  fileStat,
			}
		}
	}
}

// Progress story copy progress
type Progress struct {
	Path     string
	Total    uint64
	Current  uint64
	Error    error
	TimeLeft float64
}

func stats(info os.FileInfo, path string, p chan Progress) os.FileInfo {
	if info != nil {
		return info
	}

	info, err := os.Stat(path)
	if err != nil {
		p <- Progress{
			Path:     path,
			Total:    0,
			Current:  0,
			Error:    fmt.Errorf("Can't open file %v: %w", path, err),
			TimeLeft: 0,
		}
	}
	return info
}

// CopyFileSafely from source src to destination dst
// TODO: make this able to copy directories too
func CopyFileSafely(src FileResult, dst string, nBufferBytes uint, p chan Progress) {
	dst, err := filesys.Abs(dst)
	if err != nil {
		p <- Progress{
			Path:     dst,
			Total:    0,
			Current:  0,
			Error:    fmt.Errorf("Can't get absolute path of %v: %w", dst, err),
			TimeLeft: 0,
		}
		return
	}

	startTime := time.Now().UnixNano()

	// In case the stats are missing
	if src.Info == nil {
		src.Info = stats(src.Info, src.Path, p)
	}

	defer close(p)

	// Skip non regular files (for now)
	if !src.Info.Mode().IsRegular() {
		p <- Progress{
			Path:     dst,
			Total:    0,
			Current:  0,
			Error:    fmt.Errorf("%s is not a regular file", src),
			TimeLeft: 0,
		}
		return
	}
	var TotalSize uint64 = uint64(src.Info.Size())

	// Open the source for reading
	source, err := os.Open(src.Path)
	if err != nil {
		p <- Progress{
			Path:     dst,
			Total:    TotalSize,
			Current:  0,
			Error:    fmt.Errorf("CopyFileSafely %w", err),
			TimeLeft: 0,
		}
		return
	}

	// If the file exists
	info, err := os.Stat(dst)
	if !os.IsNotExist(err) {
		if err != nil {
			p <- Progress{
				Path:     dst,
				Total:    0,
				Current:  0,
				Error:    fmt.Errorf("Can't open file %v: %w", dst, err),
				TimeLeft: 0,
			}
			return
		}

		// Skip if the size of the target is the same as that of the source (assume it already exists and is correct)
		// TODO: Should we do a checksum check?
		if src.Info.Size() == info.Size() {
			p <- Progress{
				Path:     dst,
				Total:    TotalSize,
				Current:  TotalSize,
				Error:    nil,
				TimeLeft: 0,
			}
			return
		}

		// Error if the size of the target is larger than that of the source
		if src.Info.Size() < info.Size() {
			p <- Progress{
				Path:     dst,
				Total:    TotalSize,
				Current:  0,
				Error:    fmt.Errorf("CopyFileSafely target file \"%v\" is larger", dst),
				TimeLeft: 0,
			}
			return
		}
	}

	defer source.Close()

	tmpDst := fmt.Sprintf("%v.temp", dst)

	// Create the destination file
	destination, err := filesys.CreateFile(tmpDst)
	if err != nil {
		p <- Progress{
			Path:     dst,
			Total:    TotalSize,
			Current:  0,
			Error:    fmt.Errorf("CopyFileSafely create failed to %w", err),
			TimeLeft: 0,
		}
		return
	}
	defer destination.Close()

	var nBytes uint64
	buf := make([]byte, nBufferBytes)
	for {
		// Read a part of the source into the buffer
		n, err := source.Read(buf)
		if err != nil && err != io.EOF {
			p <- Progress{
				Path:     dst,
				Total:    TotalSize,
				Current:  0,
				Error:    err,
				TimeLeft: 0,
			}
			return
		}
		if n == 0 {
			break
		}

		// Write a part of the source from the buffer into the destination
		if _, err := destination.Write(buf[:n]); err != nil {
			p <- Progress{
				Path:     dst,
				Total:    TotalSize,
				Current:  0,
				Error:    err,
				TimeLeft: 0,
			}
			return
		}
		nBytes += uint64(n)

		// Progressbar logic
		timeSpent := uint64(time.Now().UnixNano() - startTime)
		p <- Progress{
			Path:     dst,
			Total:    TotalSize,
			Current:  nBytes,
			Error:    nil,
			TimeLeft: (float64(timeSpent) * float64(TotalSize) / float64(nBytes)) - float64(timeSpent),
		}
	}
	destination.Close()

	// Check that the checksums match, if not, error
	err = Checkcopy(src.Path, tmpDst)
	if err != nil {
		p <- Progress{
			Path:     dst,
			Total:    TotalSize,
			Current:  nBytes,
			Error:    fmt.Errorf("%w", err),
			TimeLeft: 0,
		}
		return
	}

	// Maintain the same modification and access date
	err = os.Chtimes(tmpDst, src.Info.ModTime(), src.Info.ModTime())
	if err != nil {
		p <- Progress{
			Path:     dst,
			Total:    TotalSize,
			Current:  0,
			Error:    err,
			TimeLeft: 0,
		}
		return
	}

	// Rename the temp copied file and finish the process
	err = os.Rename(tmpDst, dst)
	if err != nil {
		p <- Progress{
			Path:     dst,
			Total:    TotalSize,
			Current:  0,
			Error:    err,
			TimeLeft: 0,
		}
		return
	}

	p <- Progress{
		Path:     dst,
		Total:    TotalSize,
		Current:  nBytes,
		Error:    err,
		TimeLeft: 0,
	}
}

// CopyDirSafely ...
func CopyDirSafely(src string, dst string, nBufferBytes uint, p chan Progress) {
	src, err := filesys.Abs(src)
	if err != nil {
		p <- Progress{
			Total:    0,
			Current:  0,
			Error:    err,
			TimeLeft: 0,
		}
		return
	}

	dst, err = filesys.Abs(dst)
	if err != nil {
		p <- Progress{
			Total:    0,
			Current:  0,
			Error:    err,
			TimeLeft: 0,
		}
		return
	}

	defer close(p)
	c := make(chan FileResult)
	go WalkFilesRecursively(src, c, 10)
	for file := range c {
		if file.Error != nil {
			p <- Progress{
				Total:    0,
				Current:  0,
				Error:    file.Error,
				TimeLeft: 0,
			}
			continue
		}

		subc := make(chan Progress)
		go CopyFileSafely(file, fmt.Sprintf("%v%v", dst, strings.TrimPrefix(file.Path, src)), 64*1024*1024, subc)
		for progress := range subc {
			p <- progress
		}
	}
}

var srcPtr = flag.String("src", "test/data/source", "Source directory to copy")
var dstPtr = flag.String("dst", "test/data/target", "Destination directory to copy to")

// ./main -src="C:\Books" -dst="D:\Books"
func main() {

	flag.Parse()

	errFile, err := os.OpenFile("result/error.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		fmt.Printf("Error \"%v\"\n", err)
		return
	}
	defer errFile.Close()

	okFile, err := os.OpenFile("result/ok.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		fmt.Printf("Error \"%v\"\n", err)
		return
	}
	defer okFile.Close()

	p := make(chan Progress)
	go CopyDirSafely(*srcPtr, *dstPtr, 64*1024*1024, p)
	for progress := range p {
		if progress.Error != nil {
			fmt.Printf("Error \"%v\"\n", progress.Error)
			if _, err = errFile.WriteString(fmt.Sprintf("Error \"%v\"\n", progress.Error)); err != nil {
				panic(err)
			}
		} else {
			fmt.Printf("%v sec %v\n", progress.Path, progress.TimeLeft)
			if _, err = okFile.WriteString(fmt.Sprintf("%v sec %v\n", progress.Path, progress.TimeLeft)); err != nil {
				panic(err)
			}
		}
	}
}
