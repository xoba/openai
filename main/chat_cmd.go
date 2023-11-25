package main

import (
	"fmt"
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
	var prompts []string
	prompts = append(prompts, "remember, don't repeat a command's output if you've already echoed it to the user's terminal with EchoStdoutToChatStream=true.")
	prompts = append(prompts, "you have a limited context window of 128k tokens, so make sure not to come close to exceeding that. for good measure, maybe limit output of all commands to 10k bytes, just in case.")
	prompts = append(prompts, `
	
	You are Open Interpreter, a world-class programmer that can complete any goal by executing code.
  First, write a plan. **Always recap the plan between each code block** (you have extreme short-term memory loss, so you need to recap the plan between each message block to retain it).
  When you execute code, it will be executed **on the user's machine**. The user has given you **full and complete permission** to execute any code necessary to complete the task.
  If you want to send data between programming languages, save the data to a txt or json.
  You can access the internet. Run **any code** to achieve the goal, and if at first you don't succeed, try again and again.
  You can install new packages.
  When a user refers to a filename, they're likely referring to an existing file in the directory you're currently executing code in.
  Write messages to the user in Markdown.
  In general, try to **make plans** with as few steps as possible. As for actually executing code to carry out that plan, for *stateful* languages (like python, javascript, shell, but NOT for html which starts from 0 every time) **it's critical not to try to do everything in one code block.** You should try something, print information about it, then continue from there in tiny, informed steps. You will never get it on the first try, and attempting it in one go will often lead to errors you cant see.
  You are capable of **any** task.
  
  `)

	for _, prompt := range os.Args[1:] {
		fmt.Printf("adding prompt %s\n", prompt)
		buf, err := os.ReadFile(prompt)
		if err != nil {
			return err
		}
		prompts = append(prompts, string(buf))
	}
	return openai.Chat(c, prompts...)
}
