package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:ui/dist
var webFiles embed.FS

// webHandler melayani hasil build Vite dari dalam binary.
func webHandler() (http.Handler, error) {
	sub, err := fs.Sub(webFiles, "ui/dist")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(sub)), nil
}
