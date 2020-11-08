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
func CreateFile(path string) (*os.File, error) {
	_, err := os.Stat(path)

	if os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(path), 0755)
	}

	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return os.Create(path)
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
	root, _ = filepath.Abs(root) // TODO: handle error
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
			Error:    fmt.Errorf("Can't open file %v", path),
			TimeLeft: 0,
		}
	}
	return info
}

// CopyFileSafely from source src to destination dst
func CopyFileSafely(src FileResult, dst string, nBufferBytes uint, p chan Progress) {
	dst, _ = Abs(dst) // TODO: handle error

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

	// Error if the size of the target is larger than that of the source
	if src.Info.Size() < stats(nil, dst, p).Size() {
		p <- Progress{
			Path:     dst,
			Total:    TotalSize,
			Current:  0,
			Error:    fmt.Errorf("CopyFileSafely target file \"%v\" is larger", dst),
			TimeLeft: 0,
		}
		return
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

	// TODO: Copy all medadata
	// TODO: syscall.SetFileTime()

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
	src, _ = Abs(src) // TODO: handle error
	dst, _ = Abs(dst) // TODO: handle error

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

// ./main -src="D:\Keep\Books" -dst="E:\Keep\Books"
func main() {

	src := *flag.String("src", "test/data/deepfoldertest", "Source directory to copy")
	dst := *flag.String("dst", "test/data/deepfoldertarget", "Destination directory to copy to")

	p := make(chan Progress)
	go CopyDirSafely(src, dst, 64*1024*1024, p)
	for progress := range p {
		if progress.Error != nil {
			fmt.Printf("Error \"%v\"\n", progress.Error)
		} else {
			fmt.Printf("%v sec %v\n", progress.Path, progress.TimeLeft)
		}
	}

	// path := "D:/torrents/Prey/cpy-prey.iso"

	// sum, _ := Checksum(path)
	// fmt.Printf("Checksum %v\n", sum)

	// fileStat, err := os.Stat(path)
	// if err != nil {
	// 	log.Fatalf("Failed: %v", err)
	// 	return
	// }

	// p1 := make(chan Progress)
	// go CopyFileSafely(FileResult{"test/data/deepfoldertest/file1.txt", nil, fileStat}, "D:\\TEMP\\Prey\\file1.txt", 64*1024*1024, p1)
	// for progress := range p1 {
	// 	if progress.Error != nil {
	// 		log.Fatalf("Failed: %v", progress.Error)
	// 	}
	// 	fmt.Printf("%v %v/%v seconds left %v\n", path, progress.Current, progress.Total, progress.TimeLeft/float64(time.Second))
	// }

	// p := make(chan Progress)
	// go CopyFileSafely(FileResult{path, nil, fileStat}, "D:\\TEMP\\Prey\\prey.iso", 64*1024*1024, p)

	// for progress := range p {
	// 	if progress.Error != nil {
	// 		log.Fatalf("Failed: %v", progress.Error)
	// 	}
	// 	fmt.Printf("%v %v/%v seconds left %v\n", path, progress.Current, progress.Total, progress.TimeLeft/float64(time.Second))
	// }
}
