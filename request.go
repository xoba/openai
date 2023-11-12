package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
)

func (c Client) DoJSONRequest(method, endpoint string, in any, headers map[string]string) (*http.Response, error) {
	return c.DoRequest("POST", endpoint, in, map[string]string{
		"Content-Type": "application/json",
	})
}

func (c Client) DoRequest(method, endpoint string, in any, headers map[string]string) (*http.Response, error) {
	var u url.URL
	u.Scheme = "https"
	u.Host = "api.openai.com"
	u.Path = path.Join("v1", endpoint)
	var body io.Reader
	if in != nil {
		body = strings.NewReader(toString(in))
	}
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		io.Copy(os.Stderr, resp.Body)
		return nil, fmt.Errorf("bad status: %q", resp.Status)
	}
	if false {
		var xheaders []string
		for k := range resp.Header {
			if strings.HasPrefix(k, "X-") {
				xheaders = append(xheaders, k)
			}
		}
		sort.Strings(xheaders)
		for _, k := range xheaders {
			fmt.Printf("%s: %q\n", k, resp.Header.Get(k))
		}
	}
	return resp, nil
}
