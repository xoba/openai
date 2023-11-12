package main

import (
	"log"

	"github.com/xoba/openai"
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
