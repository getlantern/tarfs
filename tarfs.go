package tarfs

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	fileTimestamp = time.Now()
)

type FileSystem struct {
	files map[string][]byte
}

func (fs *FileSystem) Get(name string) []byte {
	return fs.files[name]
}

func New(data []byte) (*FileSystem, error) {
	fs := &FileSystem{make(map[string][]byte, 0)}

	remaining := data
	for {
		if len(remaining) == 0 {
			break
		}

		// TODO: see if we can avoid having to create a new pair of readers for
		// each file
		br := &trackingreader{bytes.NewReader(remaining), 0}
		tr := tar.NewReader(br)

		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return nil, fmt.Errorf("Unable to read next tar header: %v", err)
		}

		// Set the data to be a slice of the original
		end := br.pos + hdr.Size
		fs.files[hdr.Name] = remaining[br.pos:end]
		// Round up to multiple of 512
		end = int64(math.Ceil(float64(end)/512)) * 512

		remaining = remaining[end:]
		if err != nil {
			return nil, fmt.Errorf("Unable to seek to next header: %v", err)
		}
	}

	return fs, nil
}

func (fs *FileSystem) Open(name string) (http.File, error) {
	name = filepath.Clean(name)
	if strings.HasSuffix(name, "/") {
		fmt.Fprintf(os.Stderr, "Returning directory for %v", name)
		return NewAssetDirectory(name), nil
	}

	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	fmt.Fprintf(os.Stderr, "name: %v\n", name)
	if strings.HasSuffix(name, "/") {
		return NewAssetDirectory(name), nil
	}

	b, found := fs.files[name]
	if !found {
		return nil, fmt.Errorf("File %v not found", name)
	}
	fmt.Fprintf(os.Stderr, "Found: %v\n", name)
	fmt.Fprintln(os.Stderr, string(b))
	return NewAssetFile(name, b), nil
}

type trackingreader struct {
	*bytes.Reader

	pos int64
}

func (r *trackingreader) Read(b []byte) (int, error) {
	n, err := r.Reader.Read(b)
	r.pos += int64(n)
	return n, err
}

func (r *trackingreader) Advance(offset int64) error {
	n, err := r.Reader.Seek(offset, 1)
	if err != nil {
		return err
	}
	r.pos = n
	return nil
}

// FakeFile implements os.FileInfo interface for a given path and size
type FakeFile struct {
	// Path is the path of this file
	Path string
	// Dir marks of the path is a directory
	Dir bool
	// Len is the length of the fake file, zero if it is a directory
	Len int64
}

func (f *FakeFile) Name() string {
	_, name := filepath.Split(f.Path)
	return name
}

func (f *FakeFile) Mode() os.FileMode {
	mode := os.FileMode(0644)
	if f.Dir {
		return mode | os.ModeDir
	}
	return mode
}

func (f *FakeFile) ModTime() time.Time {
	return fileTimestamp
}

func (f *FakeFile) Size() int64 {
	return f.Len
}

func (f *FakeFile) IsDir() bool {
	return f.Mode().IsDir()
}

func (f *FakeFile) Sys() interface{} {
	return nil
}

// AssetFile implements http.File interface for a no-directory file with content
type AssetFile struct {
	*bytes.Reader
	io.Closer
	FakeFile
}

func NewAssetFile(name string, content []byte) *AssetFile {
	return &AssetFile{
		bytes.NewReader(content),
		ioutil.NopCloser(nil),
		FakeFile{name, false, int64(len(content))}}
}

func (f *AssetFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, errors.New("not a directory")
}

func (f *AssetFile) Stat() (os.FileInfo, error) {
	return f, nil
}

// AssetDirectory implements http.File interface for a directory
type AssetDirectory struct {
	AssetFile
	ChildrenRead int
	Children     []os.FileInfo
}

func NewAssetDirectory(name string) *AssetDirectory {
	fileinfos := make([]os.FileInfo, 0)
	return &AssetDirectory{
		AssetFile{
			bytes.NewReader(nil),
			ioutil.NopCloser(nil),
			FakeFile{name, true, 0},
		},
		0,
		fileinfos}
}

func (f *AssetDirectory) Readdir(count int) ([]os.FileInfo, error) {
	if count <= 0 {
		return f.Children, nil
	}
	if f.ChildrenRead+count > len(f.Children) {
		count = len(f.Children) - f.ChildrenRead
	}
	rv := f.Children[f.ChildrenRead : f.ChildrenRead+count]
	f.ChildrenRead += count
	return rv, nil
}

func (f *AssetDirectory) Stat() (os.FileInfo, error) {
	return f, nil
}
