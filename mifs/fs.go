package mifs

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/yulog/miutil"
	"gitlab.com/osaki-lab/iowrapper"
)

type FS struct {
	client *miutil.Client
}

func New(c *miutil.Client) (*FS, error) {
	return &FS{client: c}, nil
}

func (f *FS) getFile(name string) (*file, error) {
	body := map[string]any{
		"name": name,
	}
	r, err := f.client.NewPostRequest("api/drive/files/find", body)
	if err != nil {
		return nil, err
	}
	var out []miutil.File
	err = r.Do(&out)
	if err != nil {
		return nil, err
	}
	fmt.Println(out)
	resp, err := http.Get(out[0].URL)
	if err != nil {
		return nil, err
	}

	// https://github.com/golang/go/issues/27617#issuecomment-1898641407
	tee := io.TeeReader(resp.Body, io.Discard)

	// https://future-architect.github.io/articles/20201211/
	// https://gitlab.com/osaki-lab/iowrapper
	// Default buffer size: 1MB
	return &file{
		ReadSeeker: iowrapper.NewSeeker(tee, iowrapper.MaxBufferSize(100*1024*1024)),
		Closer:     resp.Body,
		fileInfo: fileInfo{
			name:    name,
			size:    int64(out[0].Size), //resp.ContentLength
			mode:    0,
			modTime: out[0].CreatedAt}}, nil
}

func (f *FS) getDir() (*dir, error) {
	body := map[string]any{}
	r, err := f.client.NewPostRequest("api/drive/files", body)
	if err != nil {
		return nil, err
	}
	var out []miutil.File
	err = r.Do(&out)
	if err != nil {
		return nil, err
	}
	return &dir{files: out}, nil
}

func (f *FS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}

	fmt.Println("Open:", name)

	// http.FileServerFSで/がindex.htmlにリダイレクトされるため
	if name == "" || name == "." || name == "index.html" {
		return f.getDir()
	}

	return f.getFile(name)
}

type file struct {
	io.ReadSeeker
	io.Closer

	fileInfo fileInfo
}

func (f *file) Stat() (fs.FileInfo, error) {
	fmt.Println(f.fileInfo)
	return &f.fileInfo, nil
}

func (f *file) Read(p []byte) (int, error) {
	return f.ReadSeeker.Read(p)
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	return f.ReadSeeker.Seek(offset, whence)
}

func (f *file) Close() error {
	return f.Closer.Close()
}

func (f *file) ReadDir(n int) ([]fs.DirEntry, error) {
	return nil, &fs.PathError{
		Op:   "read",
		Path: f.fileInfo.name,
		Err:  fs.ErrNotExist,
	}
}

type fileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (f *fileInfo) Name() string {
	return f.name
}

func (f *fileInfo) Size() int64 {
	return f.size
}

func (f *fileInfo) Mode() fs.FileMode {
	return f.mode
}

func (f *fileInfo) ModTime() time.Time {
	return f.modTime
}

func (f *fileInfo) IsDir() bool {
	return f.mode.IsDir()
}

func (f *fileInfo) Sys() any {
	return nil
}

func (f *fileInfo) Type() fs.FileMode {
	return f.Mode().Type()
}

func (f *fileInfo) Info() (fs.FileInfo, error) {
	return f, nil
}

type dir struct {
	path    string
	modTime time.Time
	files   []miutil.File
}

func (d *dir) Stat() (fs.FileInfo, error) {
	return d, nil
}

func (d *dir) Read(buf []byte) (int, error) {
	return 0, nil
}

func (d *dir) Close() error {
	return nil
}

func (d *dir) Name() string {
	return d.path
}

func (d *dir) Size() int64 {
	return 0
}

func (d *dir) Mode() fs.FileMode {
	return fs.ModeDir
}

func (d *dir) ModTime() time.Time {
	return d.modTime
}

func (d *dir) IsDir() bool {
	return true
}

func (d *dir) Sys() any {
	return nil
}

func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	var l []fs.DirEntry
	for i := range d.files {
		if n == len(l) {
			break
		}

		l = append(l, &fileInfo{
			name:    d.files[i].Name,
			size:    int64(d.files[i].Size),
			modTime: d.files[i].CreatedAt,
		})
	}

	if len(l) == 0 && n > 0 {
		return []fs.DirEntry{}, io.EOF
	}

	return l, nil
}
