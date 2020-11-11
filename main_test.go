package main

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestCommonSuffix(t *testing.T) {
	var actual string
	var expected string

	expected = "some/path"
	actual = CommonSuffix("some/path", "/some/path")
	if actual != expected {
		t.Errorf("Common path not found, expecting %v, got %v", expected, actual)
	}

	expected = "some/path"
	actual = CommonSuffix("/some/path", "some/path")
	if actual != expected {
		t.Errorf("Common path not found, expecting %v, got %v", expected, actual)
	}

	expected = "/some/path"
	actual = CommonSuffix("/some/path", "/some/path")
	if actual != expected {
		t.Errorf("Common path not found, expecting %v, got %v", expected, actual)
	}

	expected = "/file/name"
	actual = CommonSuffix("/data/test/source/file/name", "/data/test/target/file/name")
	if actual != expected {
		t.Errorf("Common path not found, expecting %v, got %v", expected, actual)
	}

	expected = "/file/name"
	actual = CommonSuffix("/data/test/source/at/some/folder/file/name", "/data/test/target/file/name")
	if actual != expected {
		t.Errorf("Common path not found, expecting %v, got %v", expected, actual)
	}

	expected = "/file/name"
	actual = CommonSuffix("/data/test/target/file/name", "/data/test/source/at/some/folder/file/name")
	if actual != expected {
		t.Errorf("Common path not found, expecting %v, got %v", expected, actual)
	}
}

func TestSplitSlugs(t *testing.T) {
	slugs := SplitSlugs("C:\\path\\to\\file")
	expected := `[]string{"C:", "path", "to", "file"}`
	actual := fmt.Sprintf("%#v", slugs)
	if actual != expected {
		t.Errorf("Path not split correctly, expecting %v, got %v", expected, actual)
	}

	slugs = SplitSlugs("C:/path/to/file")
	expected = `[]string{"C:", "path", "to", "file"}`
	actual = fmt.Sprintf("%#v", slugs)
	if actual != expected {
		t.Errorf("Path not split correctly, expecting %v, got %v", expected, actual)
	}

	slugs = SplitSlugs("/path/to/file")
	expected = `[]string{"path", "to", "file"}`
	actual = fmt.Sprintf("%#v", slugs)
	if actual != expected {
		t.Errorf("Path not split correctly, expecting %v, got %v", expected, actual)
	}
}

func TestWalkFilesRecursively(t *testing.T) {

	var files []string

	c := make(chan FileResult)
	go WalkFilesRecursively("test/data/source", c, 10)
	for file := range c {
		if MemUsage() > 280000 {
			t.Errorf("Mem too high!: %v", bToMb(MemUsage()))
		}
		files = append(files, file.Path)
	}

	expected := []string{
		"/source/file1.txt",
		"/source/P1010022.JPG",
		"/source/subfolder/file2.txt",
		"/source/subfolder/subfolder/file3.txt",
		"/source/subfolder/subfolder/file4.txt",
	}

	if len(expected) != len(files) {
		t.Errorf("Number of files was wrong, want: %v, got: %v", len(expected), len(files))
	}

	for k, expectedPath := range expected {
		actualPath, _ := Abs("test/data")
		actualPath = strings.TrimPrefix(files[k], actualPath)
		if expectedPath != actualPath {
			t.Errorf("File was wrong, want: %v, got: %v", expectedPath, actualPath)
		}
	}
}

func IsFileDateSame(src, dst string) bool {
	srcDir, err := os.Stat(src)
	if err != nil {
		panic(err)
	}

	dstDir, err := os.Stat(dst)
	if err != nil {
		panic(err)
	}

	return srcDir.ModTime() == dstDir.ModTime()
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
		t.Errorf("Wrong number of errors, expecting %v, got %v: %+v", 0, len(errors), errors)
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
		t.Errorf("Wrong number of errors, expected %v, got %v: %+v", 1, len(errors), errors)
		return
	}
	if len(files) != 1 {
		t.Errorf("Wrong number of files, expected %v, got %v", 1, len(files))
		return
	}
}

func TestCopyFileSafelyMaintainMetadata(t *testing.T) {
	var p = make(chan Progress)
	go CopyFileSafely(FileResult{"test/data/source/P1010022.JPG", nil, nil}, "test/data/target/P1010022.JPG", 1024*1024, p)
	errors := []error{}
	files := []string{}
	for progress := range p {
		if progress.Error != nil {
			errors = append(errors, progress.Error)
		}
		files = append(files, progress.Path)
	}
	if len(errors) != 0 {
		t.Errorf("Wrong number of errors, expecting %v, got %v: %+v", 0, len(errors), errors)
	}
	if len(files) < 1 {
		t.Errorf("Wrong number of files, expecting at least %v, got %v: %+v", 1, len(files), files)
	}

	srcInfo, _ := os.Stat("test/data/source/P1010022.JPG")
	dstInfo, _ := os.Stat("test/data/target/P1010022.JPG")

	if srcInfo.ModTime() != dstInfo.ModTime() {
		t.Errorf("Wrong modified date, expecting %v, got %v", srcInfo.ModTime(), dstInfo.ModTime())
	}

	os.Remove("test/data/target/P1010022.JPG")
}

func TestCopyDirectory(t *testing.T) {

	var files []string

	p := make(chan Progress)
	go CopyDirSafely("test/data/source", "test/data/target", 64*1024*1024, p)
	for progress := range p {
		files = append(files, progress.Path)
	}

	expected := []string{
		"/target/file1.txt",
		"/target/P1010022.JPG",
		"/target/subfolder/file2.txt",
		"/target/subfolder/subfolder/file3.txt",
		"/target/subfolder/subfolder/file4.txt",
	}

	for k, expectedPath := range expected {
		actualPath, _ := Abs("test/data")
		actualPath = strings.TrimPrefix(files[k], actualPath)
		expectedPath, _ := Abs(filepath.Join("test/data", expectedPath))
		if Find(files, expectedPath) == -1 {
			t.Errorf("File %v not found in files %v", expectedPath, files)
		}
	}

	if IsFileDateSame("test/data/source/subfolder", "test/data/target/subfolder") {
		t.Errorf("test/data/target/subfolder date should not match")
	}

	if !IsFileDateSame("test/data/source/subfolder/subfolder", "test/data/target/subfolder/subfolder") {
		t.Errorf("test/data/target/subfolder/subfolder date does not match")
	}

	os.Remove("test/data/target/subfolder/subfolder/file3.txt")
	os.Remove("test/data/target/subfolder/subfolder/file4.txt")
	os.Remove("test/data/target/subfolder/subfolder")
	os.Remove("test/data/target/P1010022.JPG")
}

func Find(a []string, x string) int {
	for i, n := range a {
		if x == n {
			return i
		}
	}
	return -1
}

func MemUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
