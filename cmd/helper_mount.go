//go:build linux

package cmd

import (
	"fmt"
	"os"

	"cardinal/internal/container"
)

func HelperMount(args []string) {
	if len(args) != 4 {
		fmt.Fprintln(os.Stderr, "Usage: cardinal helper-mount <lower> <upper> <work> <merged>")
		exitFunc(2)
	}
	if err := container.HelperMountDirect(args[0], args[1], args[2], args[3]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}
	fmt.Println("mounted")
}
