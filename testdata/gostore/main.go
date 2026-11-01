package main

import "fmt"

func main() {
	s := store.New()
	v, _ := s.Get("a")
	fmt.Println(v)
}
