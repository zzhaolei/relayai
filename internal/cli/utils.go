package cli

import (
	"io"
	"os"
)

func backupFile(path string) {
	src, err := os.Open(path)
	if err != nil {
		return
	}
	defer src.Close()

	dst, err := os.Create(path + ".backup")
	if err != nil {
		return
	}
	defer dst.Close()

	io.Copy(dst, src)
}
