package openai

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/google/uuid"
)

var repls = make(map[string]*StartREPL)

type StartREPL struct {
	r      *bufio.Reader
	stdout io.ReadCloser
	stdin  io.WriteCloser
	cmd    *exec.Cmd
}

func (StartREPL) Description() string {
	return `starts a python3 repl as "python3 -i -u".`
}

func (s *StartREPL) Clear() {
	*s = StartREPL{}
}

func (s StartREPL) Run() (string, error) {
	cmd := NewCommand("python3", "-i", "-u")
	s.cmd = cmd
	r, w := io.Pipe()
	cmd.Stdout = w
	cmd.Stderr = w
	p, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	s.stdout = r
	s.stdin = p
	id := uuid.NewString()[:8]
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() {
		defer r.Close()
		defer w.Close()
		if err := cmd.Wait(); err != nil {
			fmt.Printf("repl %s exited with error: %v\n", id, err)
		} else {
			fmt.Printf("repl %s exited without error\n", id)

		}
	}()
	s.r = bufio.NewReader(r)
	repls[id] = &s
	greeting := new(bytes.Buffer)
	if err := readUntil(s.r, []byte(">>> "), greeting); err != nil {
		return "", err
	}
	return greeting.String() + "\n\nrepl id = " + id, nil
}

type REPLRound struct {
	REPLId  string
	Command string
}

func (REPLRound) Description() string {
	return `does an i/o round with an existing python repl having the given repl id.`
}

func (s *REPLRound) Clear() {
	*s = REPLRound{}
}

func (s REPLRound) Run() (string, error) {
	r, ok := repls[s.REPLId]
	if !ok {
		return "", fmt.Errorf("no such repl: %s", s.REPLId)
	}
	s.Command = strings.TrimSpace(s.Command)
	if _, err := io.WriteString(r.stdin, s.Command+"\n"); err != nil {
		return "", err
	}
	w := new(bytes.Buffer)
	if err := readUntil(r.r, []byte(">>> "), w); err != nil {
		return "", err
	}
	return w.String(), nil
}

func readUntil(rr *bufio.Reader, delim []byte, w io.Writer) error {
	w = io.MultiWriter(w, os.Stdout)
	for {
		peek, err := rr.Peek(len(delim))
		if err != nil {
			return err
		}
		if bytes.Equal(peek, delim) {
			return nil
		}
		b, err := rr.ReadByte()
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte{b}); err != nil {
			return err
		}
	}
}

type StopREPL struct {
	REPLId string
}

func (StopREPL) Description() string {
	return `stops a python repl having the given repl id.`
}

func (s *StopREPL) Clear() {
	*s = StopREPL{}
}

func (s StopREPL) Run() (string, error) {
	r, ok := repls[s.REPLId]
	if !ok {
		return "", fmt.Errorf("no such repl: %s", s.REPLId)
	}
	if err := r.stdin.Close(); err != nil {
		return "", err
	}
	if err := r.stdout.Close(); err != nil {
		return "", err
	}
	if err := r.cmd.Process.Kill(); err != nil {
		return "", err
	}
	return fmt.Sprintf("repl with id %s stopped.", s.REPLId), nil
}
