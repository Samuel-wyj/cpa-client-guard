//go:build ignore

// 打包：zip 根目录只放一个动态库，并输出 sha256sum 格式校验行。
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	lib := flag.String("library", "", "")
	archive := flag.String("archive", "", "")
	flag.Parse()
	out, err := os.Create(*archive)
	check(err)
	zw := zip.NewWriter(out)
	src, err := os.Open(*lib)
	check(err)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: filepath.Base(*lib), Method: zip.Deflate})
	check(err)
	_, err = io.Copy(w, src)
	check(err)
	check(src.Close())
	check(zw.Close())
	check(out.Close())
	data, err := os.ReadFile(*archive)
	check(err)
	sum := sha256.Sum256(data)
	check(os.WriteFile(*archive+".sha256", []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), filepath.Base(*archive))), 0o644))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
