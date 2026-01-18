package main

import (
	"fmt"
	"file-sync/ui"
)

func main() {
	u := ui.NewUIManager()
	testSpeeds := []int64{0, 1, 5, 10, 100, 512, 1000, 1024, 1536, 10000, 1048576, 2147483648}

	fmt.Println("Testing formatSpeed function:")
	for _, speed := range testSpeeds {
		fmt.Printf("%d bytes/s -> %q\n", speed, u.formatSpeed(speed))
	}
}