package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"

	"github.com/binzume/dkango"
	"github.com/yulog/miutil"
	"github.com/yulog/miutil/mifs"
)

type Config struct {
	Host       string `json:"host"`
	Credential string `json:"credential"`
}

func main() {
	var c Config
	f, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Println(err)
	}
	err = json.Unmarshal(f, &c)
	if err != nil {
		fmt.Println(err)
	}
	mfs, _ := mifs.New(miutil.NewClient(c.Host, c.Credential))
	paths, err := fs.Glob(mfs, "*")
	if err != nil {
		fmt.Println(err)
	}
	for _, path := range paths {
		fmt.Println(path)
	}
	mount, err := dkango.MountFS("M:", mfs, nil)
	if err != nil {
		panic(err)
	}
	defer mount.Close()

	select {}
}
