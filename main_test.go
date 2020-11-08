package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestChecksum(t *testing.T) {
	checksum, _ := Checksum("test/data/source/file1.txt")
	if "c4ca4238a0b923820dcc509a6f75849b" != checksum {
		t.Errorf("Checksum was wrong, got: %s, expected: %s", checksum, "")
	}
}

func TestWalkFilesRecursively(t *testing.T) {

	var files []string

	c := make(chan FileResult)
	go WalkFilesRecursively("test/data/source", c, 10)
	for file := range c {
		if MemUsage() > 240000 {
			t.Errorf("Mem too high!: %v", bToMb(MemUsage()))
		}
		files = append(files, file.Path)
	}

	if 4 != len(files) {
		t.Errorf("Number of files was wrong, want: %v, got: %v", 4, len(files))
	}

	expected := []string{
		"/source/file1.txt",
		"/source/subfolder/file2.txt",
		"/source/subfolder/subfolder/file3.txt",
		"/source/subfolder/subfolder/file4.txt",
	}

	for k, expectedPath := range expected {
		actualPath, _ := Abs("test/data")
		actualPath = strings.TrimPrefix(files[k], actualPath)
		if expectedPath != actualPath {
			t.Errorf("File was wrong, want: %v, got: %v", expectedPath, actualPath)
		}
	}

	var path string
	c2 := make(chan FileResult)
	go WalkFilesRecursively("D:/torrents/Prey", c2, 10)
	for file := range c2 {
		if MemUsage() > 240000 {
			t.Errorf("Mem too high!: %v", bToMb(MemUsage()))
		}
		files = append(files, file.Path)
	}
	path = "D:/torrents/Prey/cpy-prey.iso"
	if path != files[4] {
		t.Errorf("File was wrong, want: %v, got: %v.", path, files[4])
	}
	path = "D:/torrents/Prey/cpy-prey.nfo"
	if path != files[5] {
		t.Errorf("File was wrong, want: %v, got: %v.", path, files[5])
	}
	path = "D:/torrents/Prey/README.txt"
	if path != files[6] {
		t.Errorf("File was wrong, want: %v, got: %v.", path, files[6])
	}
}

func TestCopyFileSafelyOwerwrite(t *testing.T) {
	var p = make(chan Progress)
	go CopyFileSafely(FileResult{"test/data/source/subfolder/file2.txt", nil, nil}, "test/data/target/subfolder/file2.txt", 1024*1024, p)
	errors := []error{}
	files := []string{}
	for progress := range p {
		if progress.Error != nil {
			errors = append(errors, progress.Error)
		}
		files = append(files, progress.Path)
	}
	if len(errors) != 0 {
		t.Errorf("Wrong number of errors, expecting %v, got %v", 0, len(errors))
	}
	if len(files) < 1 {
		t.Errorf("Wrong number of files, expecting at least %v, got %v: %+v", 1, len(files), files)
	}
}

func TestCopyFileSafelyNoOwerwrite(t *testing.T) {
	var p = make(chan Progress)
	go CopyFileSafely(FileResult{"test/data/source/file1.txt", nil, nil}, "test/data/target/file1.txt", 1024*1024, p)
	errors := []error{}
	files := []string{}
	for progress := range p {
		if progress.Error != nil {
			errors = append(errors, progress.Error)
		}
		files = append(files, progress.Path)
	}
	if len(errors) != 1 {
		t.Errorf("Wrong number of errors, expected %v, got %v", 1, len(errors))
		return
	}
	if len(files) != 1 {
		t.Errorf("Wrong number of files, expected %v, got %v", 1, len(files))
		return
	}
}

func MemUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
