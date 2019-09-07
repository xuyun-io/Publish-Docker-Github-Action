package publish

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

type Runner interface {
	Run() error
	StdinPipe() (io.WriteCloser, error)
}

type Publisher struct {
	Cmd func(name string, arg ...string) Runner
}

func NewPublisher() *Publisher {
	return &Publisher{
		Cmd:func(name string, arg ...string) Runner{
			return exec.Command(name, arg...)
		},
	}
}

func (p *Publisher) Run() int {
	username := os.Getenv("INPUT_USERNAME")
	password := os.Getenv("INPUT_PASSWORD")
	if os.Getenv("INPUT_NAME") == "" {
		fmt.Println("Unable to find the name. Did you set with.name?")
		return 1
	} else if username == "" {
		fmt.Println("Unable to find the username. Did you set with.username?")
		return 1
	} else if password == "" {
		fmt.Println("Unable to find the password. Did you set with.password?")
		return 1
	}

	cmd := p.Cmd("docker", "login", "-u", username, "--password-stdin")
	writer, err := cmd.StdinPipe()
	if err  != nil {
		fmt.Println(err)
		return 1
	}
	if _, err := writer.Write([]byte(password)); err  != nil {
		fmt.Println(err)
		return 1
	}
	cmd.Run()
	return 0
}
