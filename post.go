package openai

import (
	"bufio"
	"encoding/json"
	"strings"
)

func (c Client) Post(endpoint string, in, out any) error {
	resp, err := c.DoJSONRequest("POST", endpoint, in, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	d := json.NewDecoder(resp.Body)
	if err := d.Decode(&out); err != nil {
		return err
	}
	return nil
}

type StreamingChatCompletionResponse struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int               `json:"created"`
	Model   string            `json:"model"`
	Choices []StreamingChoice `json:"choices,omitempty"`
}

type StreamingChoice struct {
	Index        int    `json:"index,omitempty"`
	Delta        Delta  `json:"delta,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
}

type Delta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

func (c Client) PostStream(endpoint string, in any, cb func(StreamingChatCompletionResponse) error) ([]string, error) {
	resp, err := c.DoJSONRequest("POST", endpoint, in, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var lines []string
	reader := bufio.NewReader(resp.Body)
	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			return lines, err
		}
		trimmedInput := strings.TrimSpace(input)
		if len(trimmedInput) > 0 {
			lines = append(lines, trimmedInput)
		}
		const data = "data: "
		if strings.HasPrefix(trimmedInput, data+"[DONE]") {
			break
		}
		if strings.HasPrefix(trimmedInput, data) {
			jsonStr := strings.TrimPrefix(trimmedInput, data)
			var chatCompletion StreamingChatCompletionResponse
			err := json.Unmarshal([]byte(jsonStr), &chatCompletion)
			if err != nil {
				return lines, err
			}
			if err := cb(chatCompletion); err != nil {
				return lines, err
			}
		}
	}
	return lines, nil
}
