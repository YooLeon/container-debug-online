package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// GetFileSystem 获取嵌入的静态文件系统
func GetFileSystem() http.FileSystem {
	fsys, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}
