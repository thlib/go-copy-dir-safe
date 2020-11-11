package main

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Abs returns the absolute path of src with forward slashes
func Abs(src string) (string, error) {
	path, err := filepath.Abs(src)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(path, "\\", "/"), err
}

// CreateFile ...
func CreateFile(dst string) (*os.File, error) {
	_, err := os.Stat(dst)

	if os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(dst), 0755)
	}

	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return os.Create(dst)
}

// SplitSlugs ...
func SplitSlugs(path string) []string {
	return strings.Split(strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/"), "/")
}

// JoinPath ...
func JoinPath(path, add string) string {
	return strings.Trim(strings.Join([]string{path, add}, "/"), "/")
}

// CommonSuffix ...
func CommonSuffix(dst, src string) string {
	// First we need to find out where exactly these two paths start to differ, so we need to iterate over both paths in reverse
	dl := len(dst)
	sl := len(src)

	for i := 0; i < dl; i++ {

		// Outside the range for the source
		if i >= sl {
			return dst[dl-i:]
		}

		// Find the first character that doesn't match
		if src[sl-i-1] != dst[dl-i-1] {
			return dst[dl-i:]
		}
	}
	return dst
}

// MkdirFrom ...
func MkdirFrom(src, dst string, perm os.FileMode) error {

	src, _ = Abs(src)
	dst, _ = Abs(dst)

	srcSlugs := SplitSlugs(src)
	dstSlugs := SplitSlugs(dst)

	// path
	// path/to
	// path/to/file
	var srcPath string
	var dstPath string
	for k, dstSlug := range dstSlugs {

		// if the target already exists or doesn't then change the date of the target to be the smaller number
		// First get the date of the source

		srcPath = JoinPath(srcPath, srcSlugs[k])
		dstPath = JoinPath(dstPath, dstSlug)

		// Get the date of this path
		srcInfo, _ := os.Stat(srcPath)
		_, dstErr := os.Stat(dstPath)

		// If the directory doesn't exist, create it and modify it's modified time
		if os.IsNotExist(dstErr) {
			err := os.Mkdir(dstPath, perm)
			if err != nil {
				return err
			}

			// Change the modification date to match that of the source
			// TODO: This is pointless because as soon as we add a file the date changes
			err = os.Chtimes(dstPath, srcInfo.ModTime(), srcInfo.ModTime())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// FileResult is a data transfer struct for information about a file
type FileResult struct {
	Path  string
	Error error
	Info  os.FileInfo
}

// Checksum a file
func Checksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Checkcopy ...
func Checkcopy(src, dst string) error {
	srcCheck, err := Checksum(src)
	if err != nil {
		return fmt.Errorf("checkcopy src %v %w", src, err)
	}

	dstCheck, err := Checksum(dst)
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
	root, err := filepath.Abs(root)
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
func CopyFileSafely(src FileResult, dst string, nBufferBytes uint, p chan Progress) {
	dst, err := Abs(dst)
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
	destination, err := CreateFile(tmpDst)
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
	src, err := Abs(src)
	if err != nil {
		p <- Progress{
			Total:    0,
			Current:  0,
			Error:    err,
			TimeLeft: 0,
		}
		return
	}

	dst, err = Abs(dst)
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
