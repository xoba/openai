package openai

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/alecthomas/jsonschema"
	"github.com/google/uuid"
	"github.com/vincent-petithory/dataurl"
	"github.com/xoba/open-golang/open"
)

type Client struct {
	secretKey string
}

// new client; if secret key is empty, it will try env var OPENAI_SECRET_KEY
func NewClient(secretKey string) (*Client, error) {
	if len(secretKey) == 0 {
		secretKey = os.Getenv("OPENAI_SECRET_KEY")
	}
	return &Client{secretKey: strings.TrimSpace(secretKey)}, nil
}

// new client using secret key from file
func NewClientFilename(filename string) (*Client, error) {
	buf, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return &Client{secretKey: strings.TrimSpace(string(buf))}, nil
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int      `json:"created"`
	Model   string   `json:"model"`
	Usage   *Usage   `json:"usage,omitempty"`
	Choices []Choice `json:"choices"`
}

func checkUnique[T comparable](name string, deltas []StreamingChatCompletionResponse, f func(StreamingChatCompletionResponse) T) error {
	m := make(map[T]int)
	for _, d := range deltas {
		m[f(d)]++
	}
	if len(m) != 1 {
		return fmt.Errorf("mismatched %q: %v", name, m)
	}
	return nil
}

func streamingCombiner(deltas ...StreamingChatCompletionResponse) (*ChatCompletionResponse, error) {
	if len(deltas) == 0 {
		return nil, fmt.Errorf("no deltas")
	}
	first := deltas[0]
	if err := checkUnique("id", deltas, func(d StreamingChatCompletionResponse) string {
		return d.ID
	}); err != nil {
		return nil, err
	}
	if err := checkUnique("object", deltas, func(d StreamingChatCompletionResponse) string {
		return d.Object
	}); err != nil {
		return nil, err
	}
	if err := checkUnique("model", deltas, func(d StreamingChatCompletionResponse) string {
		return d.Model
	}); err != nil {
		return nil, err
	}
	if err := checkUnique("created", deltas, func(d StreamingChatCompletionResponse) int {
		return d.Created
	}); err != nil {
		return nil, err
	}
	if err := checkUnique("len choices", deltas, func(d StreamingChatCompletionResponse) int {
		return len(d.Choices)
	}); err != nil {
		return nil, err
	}
	if n := len(first.Choices); n != 1 {
		buf, _ := json.MarshalIndent(deltas, "", "  ")
		fmt.Println(string(buf))
		return nil, fmt.Errorf("%d choices", n)
	}
	if err := checkUnique("len toolcalls", deltas, func(d StreamingChatCompletionResponse) int {
		n := len(d.Choices[0].Delta.ToolCalls)
		if n == 0 {
			n = 1
		}
		return n
	}); err != nil {
		return nil, err
	}
	if err := checkUnique("index", deltas, func(d StreamingChatCompletionResponse) int {
		return d.Choices[0].Index
	}); err != nil {
		return nil, err
	}
	out := ChatCompletionResponse{
		ID:      first.ID,
		Object:  first.Object,
		Created: first.Created,
		Model:   first.Model,
	}
	if false {
		buf, _ := json.MarshalIndent(deltas, "", "  ")
		fmt.Println(string(buf))
	}

	toolCallsByID := make(map[string][]ToolCall)
	var lastID string
	for _, d := range deltas {
		sc := d.Choices[0]
		if len(sc.Delta.ToolCalls) > 0 {
			tc := sc.Delta.ToolCalls[0]
			if len(lastID) == 0 {
				lastID = tc.ID
			}
			if len(tc.ID) > 0 && lastID != tc.ID {
				lastID = tc.ID
			}
			toolCallsByID[lastID] = append(toolCallsByID[lastID], tc)
		}
	}

	var c Choice
	for k, v := range toolCallsByID {
		tc := ToolCall{
			ID: k,
		}
		for _, x := range v {
			if x.Type != "" {
				tc.Type = x.Type
			}
			if x.FunctionCall.Name != "" {
				tc.FunctionCall.Name = x.FunctionCall.Name
			}
			tc.FunctionCall.Arguments += x.FunctionCall.Arguments
		}
		c.Message.ToolCalls = append(c.Message.ToolCalls, tc)
	}

	for _, d := range deltas {
		sc := d.Choices[0]
		if len(sc.Delta.Role) > 0 {
			c.Message.Role = sc.Delta.Role
		}
		c.Message.Content += sc.Delta.Content
		c.FinishReason = sc.FinishReason
	}

	out.Choices = []Choice{c}
	if false {
		buf, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(buf))
	}
	return &out, nil
}

type Choice struct {
	Index        int     `json:"index,omitempty"`
	Message      Message `json:"message,omitempty"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // in replying with function response
	Name       string     `json:"name,omitempty"`         // in replying with function response

	//Content    []Content  `json:"content,omitempty"` --- alternate Content struct needed for gpt4v images
}

type FunctionI interface {
	Description() string
	Clear()
	Run() (string, error)
}

type FileCreation struct {
	Filename    string
	UTF8Content string
}

func (FileCreation) Description() string {
	return "creates a file with given name and content, which is better than echo'ing or redirecting into a file because it can handle special characters, line newline escapes etc."
}

func (s *FileCreation) Clear() {
	*s = FileCreation{}
}

func (s FileCreation) Run() (string, error) {
	if err := os.WriteFile(s.Filename, []byte(s.UTF8Content), os.ModePerm); err != nil {
		return "", err
	}
	return fmt.Sprintf("created file %q with %d bytes content", s.Filename, len(s.UTF8Content)), nil
}

type RandomJoke struct {
}

func (RandomJoke) Description() string {
	return "fetches a random joke"
}

func (s *RandomJoke) Clear() {
	*s = RandomJoke{}
}

func (s RandomJoke) Run() (string, error) {
	return loadJoke()
}

type TextSorter struct {
	Lines []string
}

func (TextSorter) Description() string {
	return "sorts lines of text in lexical order."
}

func (s *TextSorter) Clear() {
	*s = TextSorter{}
}

func (s TextSorter) Run() (string, error) {
	sort.Strings(s.Lines)
	buf, _ := json.MarshalIndent(s.Lines, "", "  ")
	return string(buf), nil
}

type NumberSorter struct {
	Lines []float64
}

func (NumberSorter) Description() string {
	return "sorts numbers."
}

func (s *NumberSorter) Clear() {
	*s = NumberSorter{}
}

func (s NumberSorter) Run() (string, error) {
	sort.Float64s(s.Lines)
	buf, _ := json.MarshalIndent(s.Lines, "", "  ")
	return string(buf), nil
}

type SummationRequest struct {
	Summands []float64
}

func (SummationRequest) Description() string {
	return "adds numbers together."
}

func (s *SummationRequest) Clear() {
	*s = SummationRequest{}
}

func (s SummationRequest) Run() (string, error) {
	var sum float64
	for _, i := range s.Summands {
		sum += i
	}
	return fmt.Sprintf("%f", sum), nil
}

type ProductRequest struct {
	Factors []float64
}

func (ProductRequest) Description() string {
	return "multiplies numbers together."
}

func (s *ProductRequest) Clear() {
	*s = ProductRequest{}
}

func (s ProductRequest) Run() (string, error) {
	product := 1.0
	for _, i := range s.Factors {
		product *= i
	}
	return fmt.Sprintf("%f", product), nil
}

type SquareRoot struct {
	Argument float64
}

func (SquareRoot) Description() string {
	return "takes the square root of a number."
}

func (s *SquareRoot) Clear() {
	*s = SquareRoot{}
}

func (s SquareRoot) Run() (string, error) {
	return fmt.Sprintf("%f", math.Sqrt(s.Argument)), nil
}

type Command struct {
	Line string
}

func (Command) Description() string {
	return `runs a command using "bash -c ....", and returns stdout, stderr, exit code, etc.`
}

func (s *Command) Clear() {
	*s = Command{}
}

func (s Command) Run() (string, error) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := exec.Command("bash", "-c", s.Line)
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	var runError string
	if err := cmd.Run(); err != nil {
		runError = err.Error()
	}
	w := new(bytes.Buffer)
	e := json.NewEncoder(w)
	e.SetEscapeHTML(false)
	if err := e.Encode(map[string]any{
		"error":               runError,
		"exit_code":           cmd.ProcessState.ExitCode(),
		"success":             cmd.ProcessState.Success(),
		"system_time_seconds": cmd.ProcessState.SystemTime().Seconds(),
		"user_time_seconds":   cmd.ProcessState.UserTime().Seconds(),
		"stderr":              stderr.String(),
		"stdout":              stdout.String(),
	}); err != nil {
		return "", err
	}
	return w.String(), nil
}

type ChatRequest struct {
	Model          string          `json:"model"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Temperature    float64         `json:"temperature"`
	Stream         bool            `json:"stream,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	Messages       []Message       `json:"messages"`
	Tools          []Tool          `json:"tools,omitempty"`
}

type Tool struct {
	Type     string    `json:"type"`
	Function *Function `json:"function,omitempty"`
}

func functionDefinition(i FunctionI) *Function {
	return &Function{
		Name:        structName(i),
		Description: i.Description(),
		Parameters:  schema(i),
	}
}

type Function struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

func NoParameters() any {
	var m map[string]any
	if err := json.Unmarshal([]byte(`{"type": "object", "properties": {}}`), &m); err != nil {
		panic(err)
	}
	return m
}

func (a ChatRequest) String() string {
	return toString(a)
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type DalleResponse struct {
	Created int
	Data    []DalleData
}

type DalleData struct {
	URL           string `json:"url,omitempty"`
	B64Json       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

func (d DalleResponse) String() string {
	return toString(d)
}

func dalle(c *Client) error {
	var out DalleResponse
	if err := c.Post("images/generations", ImageRequest{
		Prompt:         "show me the ocean in coney island, with the beach, bathers, amusement park, and with an airplane in the distance. show the full moon in the sky.",
		Model:          "dall-e-3",
		ResponseFormat: "b64_json",
		Quality:        "hd",
	}, &out); err != nil {
		return err
	}
	for _, x := range out.Data {
		if b := x.B64Json; len(b) > 0 {
			buf, err := base64.StdEncoding.DecodeString(b)
			if err != nil {
				return fmt.Errorf("can't decode b64: %w", err)
			}
			mimeType := http.DetectContentType(buf)
			ext, err := mime.ExtensionsByType(mimeType)
			if err != nil {
				return err
			}
			n := uuid.NewString() + ext[0]
			if err := os.WriteFile(n, buf, os.ModePerm); err != nil {
				return err
			}
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			var u url.URL
			u.Scheme = "file"
			u.Path = filepath.Join(wd, n)
			x.URL = u.String()
		}
		fmt.Println(x.RevisedPrompt)
		if err := open.Run(x.URL); err != nil {
			return err
		}
	}
	return nil
}

type ModelResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func (r *ModelResponse) String() string {
	return toString(r)
}

func LoadModels(c *Client) (*ModelResponse, error) {
	var list ModelResponse
	if err := c.Get("models", &list); err != nil {
		return nil, err
	}
	return &list, nil
}

func loadImage(filename string) (*Content, error) {
	xoba, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	u := dataurl.New(xoba, "image/jpeg")
	if false {
		fmt.Println(u)
	}
	return &Content{
		Type:     "image_url",
		ImageURL: u.String(),
	}, nil
}

func loadJoke() (string, error) {
	f, err := os.Open("jokes.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	var list []string
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if len(line) == 0 {
			continue
		}
		list = append(list, strings.ReplaceAll(line, "<>", " ... "))
	}
	return list[rand.Intn(len(list))], nil
}

func (c *Client) Get(endpoint string, out any) error {
	resp, err := c.DoRequest("GET", endpoint, nil, nil)
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

func (c StreamingChatCompletionResponse) String() string {
	return toString(c)
}

func (s ChatCompletionResponse) String() string {
	return toString(s)
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ToolCall struct {
	ID           string       `json:"id"`
	Type         string       `json:"type"`
	FunctionCall FunctionCall `json:"function"`
}

func (t ToolCall) String() string {
	return toString(t)
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func toString(i interface{}) string {
	w := new(bytes.Buffer)
	e := json.NewEncoder(w)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	e.Encode(i)
	return w.String()
}

type ImageRequest struct {
	Prompt         string `json:"prompt"`
	Model          string `json:"model,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Size           string `json:"size,omitempty"`
	Style          string `json:"style,omitempty"`
}

type SumArgs struct {
	A, B float64
}

func structName(i any) string {
	return reflect.TypeOf(i).Elem().Name()
}

func schema(i any) *jsonschema.Type {
	s := jsonschema.Reflect(i).Definitions[structName(i)]
	elem := reflect.ValueOf(i).Elem()
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Type().Field(i)
		if tag, ok := field.Tag.Lookup("enum"); ok {
			var list []interface{}
			for _, v := range strings.Split(tag, ",") {
				list = append(list, v)
			}
			p, ok := s.Properties.Get(field.Name)
			if !ok {
				panic("no " + field.Name)
			}
			p.(*jsonschema.Type).Enum = list
			s.Properties.Set(field.Name, p)
		}
		if tag, ok := field.Tag.Lookup("desc"); ok {
			p, ok := s.Properties.Get(field.Name)
			if !ok {
				s.Description = tag
				continue
			}
			p.(*jsonschema.Type).Description = tag
			s.Properties.Set(field.Name, p)
		}
	}
	s.AdditionalProperties = nil
	return s
}
