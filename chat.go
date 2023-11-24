package openai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func Chat(c *Client, prompts ...string) error {
	var messages []Message
	messages = append(messages, Message{
		Role: "system",
		Content: `you are a helpful assistant. 
		if the user asks about his machine or system, you have a function which can execute commands and should use it safely without harming his system.
		generally prefer using python3 for any calculations, running scripts you first create (or edit) in the /tmp/ filesystem. install whatever libraries
		are necessary. when running commands, you can optionally echo the command's output to the user's terminal,
		in which case, there's no need to repeat that output yourself.
		`,
	})
	for _, p := range prompts {
		messages = append(messages, Message{
			Role:    "user",
			Content: p,
		})
	}
	o := ChatOptions{
		Functions:      StandardFuncs(),
		Messages:       messages,
		Model:          "gpt-4-1106-preview",
		ResponseFormat: "text",
	}
	if err := ChatWithOptions(c, &o); err != nil {
		return err
	}
	fmt.Println()
	for _, m := range o.Messages {
		fmt.Printf("%s: %s\n", m.Role, m.Content)
	}
	return nil
}

func StandardFuncs() (out []FunctionI) {
	add := func(f FunctionI) {
		out = append(out, f)
	}
	add(&SummationRequest{})
	add(&ProductRequest{})
	add(&Command{})
	add(&SquareRoot{})
	add(&RandomJoke{})
	add(&FictionalMessage{})
	add(&TextSorter{})
	add(&NumberSorter{})
	add(&FileCreation{})
	add(&StartREPL{})
	add(&REPLRound{})
	add(&StopREPL{})
	add(&YoutubeViewModel{})
	return
}

type ChatOptions struct {
	Functions      []FunctionI
	Messages       []Message
	Model          string
	OneRound       bool
	ResponseFormat string // text or json_object
}

func ChatWithOptions(c *Client, o *ChatOptions) error {

	var reader *bufio.Reader

	if !o.OneRound {
		reader = bufio.NewReader(os.Stdin)
	}

	var toolCall bool

	funcs := make(map[string]FunctionI)
	add := func(f FunctionI) {
		fd := functionDefinition(f)
		if false {
			buf, _ := json.MarshalIndent(fd.Parameters, "", "  ")
			fmt.Printf("adding function %s: %s\n", fd.Name, string(buf))
		}
		name := fd.Name
		if _, ok := funcs[name]; ok {
			panic("duplicate: " + name)
		}
		funcs[name] = f
	}
	for _, f := range o.Functions {
		add(f)
	}

	for {

		if false {
			buf, _ := json.MarshalIndent(o.Messages, "", "  ")
			fmt.Printf("messages: %s\n", string(buf))
		}

		if !toolCall && !o.OneRound {
			fmt.Print("> ")
			text, err := reader.ReadString('\n')
			if err == io.EOF {
				fmt.Println()
				break
			} else if err != nil {
				return fmt.Errorf("can't read from stdin: %v", err)
			}
			text = strings.TrimSpace(text)

			if len(text) == 0 {
				continue
			}

			o.Messages = append(o.Messages, Message{
				Role:    "user",
				Content: text,
			})
		}

		toolCall = false

		chatRequest := ChatRequest{
			Stream: true,
			ResponseFormat: &ResponseFormat{
				Type: o.ResponseFormat,
			},
			//Model: "gpt-4",
			//Model: "gpt-4-vision-preview",
			//Model:       "gpt-4-1106-preview",
			Model:       o.Model,
			Messages:    o.Messages,
			Temperature: 0.7,
		}

		for _, f := range funcs {
			chatRequest.Tools = append(chatRequest.Tools, Tool{
				Type:     "function",
				Function: functionDefinition(f),
			})
		}

		const endpoint = "chat/completions"

		var r *ChatCompletionResponse
		var deltas []StreamingChatCompletionResponse
		if chatRequest.Stream {
			lines, err := c.PostStream(endpoint, chatRequest, func(c StreamingChatCompletionResponse) error {
				if len(c.Choices) > 0 {
					fmt.Print(c.Choices[0].Delta.Content)
					if tc := c.Choices[0].Delta.ToolCalls; len(tc) > 0 {
						fmt.Print(tc[0].FunctionCall.Name)
						fmt.Print(tc[0].FunctionCall.Arguments)
					}
				}
				deltas = append(deltas, c)
				return nil
			})
			if err != nil {
				return err
			}
			x, err := streamingCombiner(deltas...)
			if err != nil {
				for i, x := range lines {
					fmt.Printf("line %d: %s\n", i, x)
				}
				return err
			}
			r = x
		} else {
			if err := c.Post(endpoint, chatRequest, &r); err != nil {
				return err
			}
		}

		if len(r.Choices) != 1 {
			{
				buf, _ := json.MarshalIndent(deltas, "", "  ")
				fmt.Println("deltas:", string(buf))
			}
			{
				buf, _ := json.MarshalIndent(r, "", "  ")
				fmt.Println("combined:", string(buf))
			}
			return fmt.Errorf("bad number of choices: %d", len(r.Choices))
		}

		choice := r.Choices[0]
		o.Messages = append(o.Messages, choice.Message)
		if o.OneRound {
			return nil
		}
		switch choice.FinishReason {
		case "tool_calls":
			toolCall = true
			for _, t := range choice.Message.ToolCalls {
				f, ok := funcs[t.FunctionCall.Name]
				if !ok {
					return fmt.Errorf("unknown func: %q", t.FunctionCall.Name)
				}
				f.Clear()
				if err := json.Unmarshal([]byte(t.FunctionCall.Arguments), f); err != nil {
					return fmt.Errorf("%w: can't parse arguments of %q --- %s", err, t.FunctionCall.Name, t.FunctionCall.Arguments)
				}
				c, err := f.Run()
				if err != nil {
					return fmt.Errorf("can't run %q: %w", t.FunctionCall.Name, err)
				}
				o.Messages = append(o.Messages, Message{
					Role:       "tool",
					ToolCallID: t.ID,
					Name:       t.FunctionCall.Name,
					Content:    c,
				})
			}
		default:
			content := choice.Message.Content
			if !chatRequest.Stream {
				fmt.Print(content)
			}
		}
		fmt.Println()
	}

	return nil
}
