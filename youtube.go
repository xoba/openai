package openai

import (
	"fmt"
	"math"
	"strings"
)

type YoutubeViewModel struct {
	VideoTitle       string
	VideoDescription string
}

func (YoutubeViewModel) Description() string {
	return "predicts how many views a youtube video will get, based on the title and description."
}

func (s *YoutubeViewModel) Clear() {
	*s = YoutubeViewModel{}
}

func (s YoutubeViewModel) Run() (string, error) {
	var logv float64
	wc := func(s string) float64 {
		return float64(len(strings.Fields(s)))
	}
	// simple dumb model: based on wordcounts alone
	logv += math.Log(wc(s.VideoDescription))
	logv -= math.Log(wc(s.VideoTitle))
	return fmt.Sprintf("%.0f views predicted", math.Exp(10+logv)), nil
}
