package main

import "fmt"

import "example4/arch"
import "example4/os"

func main() {
	fmt.Printf("Hello from ")

	var list [2]string
	list[0] = arch.Name
	list[1] = os.Name
	for i, s := range list {
		if len(s) == 0 {
			continue
		}
		if i > 0 {
			fmt.Printf(" and ")
		}
		fmt.Printf("%s", s)
	}

	fmt.Printf("\n")
}