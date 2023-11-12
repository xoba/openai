package main

import (
	"log"

	"xoba.com/openai"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	c, err := openai.NewClient("")
	if err != nil {
		return err
	}
	return openai.Chat(c)
}
