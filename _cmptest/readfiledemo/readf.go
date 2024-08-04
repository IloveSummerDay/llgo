package main

import (
	"fmt"
	"os"
)

func main() {
	fileName := "err.log"
	os.WriteFile(fileName, []byte("123"), 0644)

	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "File %s not found\n", fileName)
		return
	}

	data, err := os.ReadFile(fileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadFile: %v\n", err)
		return
	}

	fmt.Printf("%s\n", data)
}
