// Command fixturegen creates deterministic, visually distinct PNG fixtures for
// local end-to-end acceptance without downloading or mutating user data.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	output := flag.String("output", "runtime/imports/unique-100", "fixture output directory")
	count := flag.Int("count", 100, "number of fixtures")
	flag.Parse()
	if *count < 1 || *count > 10000 {
		fmt.Fprintln(os.Stderr, "count must be between 1 and 10000")
		os.Exit(2)
	}
	if err := os.MkdirAll(*output, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for index := 0; index < *count; index++ {
		if err := writeFixture(*output, index); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Printf("generated %d deterministic PNG fixtures in %s\n", *count, *output)
}

func writeFixture(output string, index int) error {
	const width, height = 683, 538
	imageData := image.NewRGBA(image.Rect(0, 0, width, height))
	background := color.RGBA{
		R: uint8(26 + index*31%150),
		G: uint8(35 + index*47%150),
		B: uint8(55 + index*61%150),
		A: 255,
	}
	draw.Draw(imageData, imageData.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)
	accent := color.RGBA{R: uint8(85 + index*19%170), G: uint8(110 + index*23%145), B: uint8(135 + index*29%120), A: 255}
	box := image.Rect(80+index%17, 70+index%23, 420+index%71, 330+index%83)
	draw.Draw(imageData, box, &image.Uniform{C: accent}, image.Point{}, draw.Src)
	for bit := 0; bit < 16; bit++ {
		if index&(1<<bit) == 0 {
			continue
		}
		x := 35 + bit*35
		draw.Draw(imageData, image.Rect(x, 460, x+22, 505), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	}
	path := filepath.Join(output, fmt.Sprintf("defect-%03d.png", index+1))
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err = png.Encode(file, imageData); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
