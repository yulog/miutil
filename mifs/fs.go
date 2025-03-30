package mifs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/patrickmn/go-cache"
	"github.com/yulog/miutil"
	"gitlab.com/osaki-lab/iowrapper"
	"golang.org/x/sync/singleflight"
)

var (
	_ fs.FS          = (*FS)(nil)
	_ fs.File        = (*file)(nil)
	_ io.Seeker      = (*file)(nil)
	_ fs.FileInfo    = (*fileInfo)(nil)
	_ fs.DirEntry    = (*fileInfo)(nil)
	_ fs.ReadDirFile = (*dir)(nil)
)

type FS struct {
	client *miutil.Client

	cache     *bigcache.BigCache
	fileCache *cache.Cache
}

func New(c *miutil.Client) (*FS, error) {
	config := bigcache.DefaultConfig(10 * time.Second)
	config.HardMaxCacheSize = 10 // MB
	bcache, err := bigcache.New(context.TODO(), config)
	if err != nil {
		return nil, err
	}
	fcache := cache.New(5*time.Minute, 10*time.Minute)
	return &FS{client: c, cache: bcache, fileCache: fcache}, nil
}

func (f *FS) doWithCache(path string, r *miutil.Request, out any) error {
	hash := fnv.New64a()
	hash.Write([]byte(path))
	hash.Write([]byte(r.URL))
	var b io.Reader = r.Body
	r.Body = io.TeeReader(b, hash)
	sum := hash.Sum64()
	if e, err := f.cache.Get(fmt.Sprintf("%x", sum)); err == nil {
		fmt.Println("Cache Hit:", path, sum)
		err := gob.NewDecoder(bytes.NewBuffer(e)).Decode(out)
		if err != nil {
			return err
		}
	} else {
		fmt.Println("Cache Not Hit:", path)
		err := r.Do(out)
		if err != nil {
			return err
		}
		buf := bytes.NewBuffer(nil)
		err = gob.NewEncoder(buf).Encode(out)
		if err != nil {
			return err
		}
		f.cache.Set(fmt.Sprintf("%x", sum), buf.Bytes())
	}
	return nil
}

func (f *FS) getFile(name string) (*file, error) {
	dir, name := path.Split(name)
	id := ""
	if dir != "" {
		ok, folder := f.findDir(dir)
		fmt.Println(dir, ok, folder)
		if !ok {
			return nil, &fs.PathError{
				Op:   "open",
				Path: name,
				Err:  fs.ErrNotExist,
			}
		}
		id = folder.ID
	}

	body := map[string]any{
		"name": name,
	}
	if id != "" {
		body["folderId"] = id
	}
	r, err := f.client.NewPostRequest("api/drive/files/find", body)
	if err != nil {
		return nil, err
	}
	var out []miutil.File
	err = f.doWithCache(name, r, &out)
	if err != nil {
		return nil, err
	}
	fmt.Println(out)
	if len(out) == 0 {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}
	var resp *http.Response
	req, _ := http.NewRequest(http.MethodGet, out[0].URL, nil)
	v, found := f.fileCache.Get(out[0].URL)
	if found {
		fmt.Println("FileCache Hit:", out[0].URL)
		resp, err = http.ReadResponse(bufio.NewReader(bytes.NewReader(v.([]byte))), req)
		if err != nil {
			return nil, err
		}
	} else {
		fmt.Println("FileCache Not Hit:", out[0].URL)
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println("do", err)
			return nil, err
		}
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			fmt.Println("dump", err)
			return nil, err
		}
		f.fileCache.Set(out[0].URL, dump, cache.DefaultExpiration)
		fmt.Println("set", len(dump))
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
			Fmode:   0,
			modTime: out[0].CreatedAt}}, nil
}

var dirGroup singleflight.Group

func (f *FS) getDir(fi fileInfo) (*dir, error) {
	var d dir
	var out []miutil.File
	var out2 []miutil.Folder
	if e, err := f.cache.Get(fi.name); err == nil {
		fmt.Println("Hit:", fi.name)
		err := gob.NewDecoder(bytes.NewBuffer(e)).Decode(&d)
		if err != nil {
			return nil, err
		}
		return &d, nil
	}

	var r *miutil.Request
	var err error
	if fi.name == "" || fi.name == "." {
		fmt.Println("Call:", fi.name)
		r, err := f.client.NewPostRequest("api/drive/files", map[string]any{})
		if err != nil {
			return nil, err
		}
		err = f.doWithCache(fi.name, r, &out)
		if err != nil {
			return nil, err
		}

		r, err = f.client.NewPostRequest("api/drive/folders", map[string]any{})
		if err != nil {
			return nil, err
		}
		err = f.doWithCache(fi.name, r, &out2)
		if err != nil {
			return nil, err
		}
	} else {
		ok, dir := f.findDir(fi.name)
		if !ok {
			return nil, err
		}

		r, err = f.client.NewPostRequest("api/drive/files", map[string]any{"folderId": dir.ID})
		if err != nil {
			return nil, err
		}
		err = f.doWithCache(fi.name, r, &out)
		if err != nil {
			return nil, err
		}

		r, err = f.client.NewPostRequest("api/drive/folders", map[string]any{"folderId": dir.ID})
		if err != nil {
			return nil, err
		}
		err = f.doWithCache(fi.name, r, &out2)
		if err != nil {
			return nil, err
		}
	}

	d = dir{F: fi, modTime: fi.modTime, Files: out, Folders: out2}

	buf := bytes.NewBuffer(nil)
	err = gob.NewEncoder(buf).Encode(&d)
	if err != nil {
		return nil, err
	}
	f.cache.Set(fi.name, buf.Bytes())
	fmt.Println("getDir dir:", d)

	return &d, nil
}

func (f *FS) dirExists(name string) (bool, fileInfo) {
	if name == "" || name == "." {
		return true, fileInfo{name: name, Fmode: fs.ModeDir, modTime: time.Time{}}
	}
	ok, dir := f.findDir(name)
	if !ok {
		return false, fileInfo{}
	}

	return ok, fileInfo{name: name, Fmode: fs.ModeDir, modTime: dir.CreatedAt}
}

func (f *FS) findDir(path string) (bool, miutil.Folder) {
	var out []miutil.Folder
	var pid string
	var r *miutil.Request
	var err error
	ps := strings.Split(strings.Trim(path, "/"), "/")
	for i, v := range ps {
		if i < 1 {
			r, err = f.client.NewPostRequest("api/drive/folders/find", map[string]any{"name": v})
		} else {
			r, err = f.client.NewPostRequest("api/drive/folders/find", map[string]any{"name": v, "parentId": pid})
		}
		if err != nil {
			return false, miutil.Folder{}
		}
		err = f.doWithCache(v, r, &out)
		if err != nil {
			return false, miutil.Folder{}
		}
		ok := slices.ContainsFunc(out, func(e miutil.Folder) bool {
			return e.Name == v
		})
		if !ok {
			return false, miutil.Folder{}
		}
		pid = out[0].ID
	}
	return true, out[0]
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
	bol, de := f.dirExists(name)
	fmt.Println("dir ?:", bol, de.Fmode)
	if bol {
		// TODO: singleflightの使い方が分かっていない
		v, err, shared := dirGroup.Do(name, func() (any, error) {
			return f.getDir(de)
		})
		if err != nil {
			return nil, err
		}
		// fmt.Printf("result: %s, shared: %t\n", v, shared)
		fmt.Printf("shared: %t\n", shared)
		return v.(*dir), nil
	}

	return f.getFile(name)
}

type file struct {
	io.ReadSeeker
	io.Closer

	fileInfo fileInfo
}

func (f *file) Stat() (fs.FileInfo, error) {
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
	Fmode   fs.FileMode // use gob
	modTime time.Time
}

func (f *fileInfo) Name() string {
	return f.name
}

func (f *fileInfo) Size() int64 {
	return f.size
}

func (f *fileInfo) Mode() fs.FileMode {
	return f.Fmode
}

func (f *fileInfo) ModTime() time.Time {
	return f.modTime
}

func (f *fileInfo) IsDir() bool {
	return f.Fmode.IsDir()
}

func (f *fileInfo) Sys() any {
	return nil
}

// Type for fs.DirEntry
func (f *fileInfo) Type() fs.FileMode {
	return f.Mode().Type()
}

// Info for fs.DirEntry
func (f *fileInfo) Info() (fs.FileInfo, error) {
	return f, nil
}

type dir struct {
	F       fileInfo // use gob
	modTime time.Time
	Files   []miutil.File   // use gob
	Folders []miutil.Folder // use gob
}

func (d *dir) Stat() (fs.FileInfo, error) {
	return &d.F, nil
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
	for i := range d.Files {
		if n == len(l) {
			break
		}

		l = append(l, &fileInfo{
			name:    d.Files[i].Name,
			size:    int64(d.Files[i].Size),
			modTime: d.Files[i].CreatedAt,
		})
	}
	d.Files = nil
	for i := range d.Folders {
		if n == len(l) {
			break
		}

		l = append(l, &fileInfo{
			name:    d.Folders[i].Name,
			size:    0,
			Fmode:   fs.ModeDir,
			modTime: d.Folders[i].CreatedAt,
		})
	}
	d.Folders = nil

	if len(l) == 0 && n > 0 {
		return []fs.DirEntry{}, io.EOF
	}

	return l, nil
}
