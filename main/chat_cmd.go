package main

import (
	"log"
	"os"
	"strings"

	"github.com/xoba/openai"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func loadKey() (string, error) {
	buf, err := os.ReadFile("openai_personal.txt")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(buf)), nil
}

func run() error {
	key, err := loadKey()
	if err != nil {
		return err
	}
	c, err := openai.NewClient(key)
	if err != nil {
		return err
	}
	return openai.Chat(c)
}
