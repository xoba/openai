package openai

import (
	"fmt"
	"io"
)

type Response struct {
	Content      string
	FinishReason string
}

func (c *Client) Streaming(model string, messages []Message, stream io.Writer) (*Response, error) {
	chatRequest := ChatRequest{
		Stream: true,
		ResponseFormat: &ResponseFormat{
			Type: "text",
		},
		Model:       model,
		Messages:    messages,
		Temperature: 0.7,
	}
	const endpoint = "chat/completions"
	var r *ChatCompletionResponse
	var deltas []StreamingChatCompletionResponse
	if chatRequest.Stream {
		if _, err := c.PostStream(endpoint, chatRequest, func(c StreamingChatCompletionResponse) error {
			if len(c.Choices) > 0 {
				if _, err := stream.Write([]byte(c.Choices[0].Delta.Content)); err != nil {
					return err
				}
			}
			deltas = append(deltas, c)
			return nil
		}); err != nil {
			return nil, err
		}
		x, err := streamingCombiner(deltas...)
		if err != nil {
			return nil, err
		}
		r = x
	} else {
		if err := c.Post(endpoint, chatRequest, &r); err != nil {
			return nil, err
		}
	}
	if len(r.Choices) != 1 {
		return nil, fmt.Errorf("bad number of choices: %d", len(r.Choices))
	}
	choice := r.Choices[0]
	messages = append(messages, choice.Message)
	return &Response{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
	}, nil
}
