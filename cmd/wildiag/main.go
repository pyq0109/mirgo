package main

import (
	"fmt"
	"image/png"
	"os"
	"strconv"

	"github.com/pyq0109/mirgo/internal/wil"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: wildiag <wil-path> <image-index> [output.png]")
		os.Exit(1)
	}
	path := os.Args[1]
	idx, _ := strconv.Atoi(os.Args[2])
	outPath := fmt.Sprintf("diag_%d.png", idx)
	if len(os.Args) >= 4 {
		outPath = os.Args[3]
	}

	f, err := wil.Load(path)
	if err != nil {
		fmt.Printf("load error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	img := f.GetImage(idx)
	if img == nil || img.RGBA == nil {
		fmt.Printf("Image %d: nil RGBA\n", idx)
		return
	}

	fmt.Printf("Image %d: %dx%d HotX=%d HotY=%d\n", idx, img.Width, img.Height, img.HotX, img.HotY)

	out, err := os.Create(outPath)
	if err != nil {
		fmt.Printf("create file: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	if err := png.Encode(out, img.RGBA); err != nil {
		fmt.Printf("png encode: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("saved to %s\n", outPath)
}
