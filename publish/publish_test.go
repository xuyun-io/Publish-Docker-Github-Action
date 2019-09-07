package publish_test

import (
	"bytes"
	"fmt"
	"github.com/elgohr/Publish-Docker-Github-Action/publish"
	"github.com/kami-zh/go-capturer"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestErrorsWhenMandatoryInputsAreNotSet(t *testing.T) {
	defer unsetMandatoryVariables(t)
	p := publish.NewPublisher()

	for i, input := range mandatoryInputs() {
		t.Run(input, func(t *testing.T) {
			for j, input := range mandatoryInputs() {
				if i != j {
					assert.Nil(t,  os.Setenv("INPUT_"+strings.ToUpper(input), input))
				}
			}
			var res int
			out := strings.TrimSpace(capturer.CaptureStdout(
				func() {
					res = p.Run()
				}))
			assert.Equal(t, 1, res, "should have errored")
			expStdout := fmt.Sprintf("Unable to find the %v. Did you set with.%v?", input, input)
			assert.Equal(t, expStdout, out)

			for _, input := range mandatoryInputs() {
				assert.Nil(t, os.Unsetenv("INPUT_" + strings.ToUpper(input)))
			}
		})
	}
}

func TestLogsIntoDockerRegistry(t *testing.T) {
	setMandatoryVariables(t)
	defer unsetMandatoryVariables(t)

	var ran bool

	p := publish.NewPublisher()
	p.Cmd = func(name string, arg ...string) publish.Runner {
		assert.Equal(t, "docker", name)
		assert.Equal(t, strings.Split("login -u username --password-stdin", " "), arg)
		return &FakeRunner{
			Mock: func() error {
				ran = true
				return nil
		}}
	}
	if res :=p.Run(); res != 0 {
		t.Errorf("Should have returned 0, but was %v", res)
	}

	assert.True(t, ran)
}

func setMandatoryVariables(t *testing.T) {
	for _, input := range mandatoryInputs() {
		if err := os.Setenv("INPUT_"+strings.ToUpper(input), input); err != nil {
			t.Error(err)
		}
	}
}

func unsetMandatoryVariables(t *testing.T) {
	for _, input := range mandatoryInputs() {
		if err := os.Unsetenv("INPUT_"+strings.ToUpper(input)); err != nil {
			t.Error(err)
		}
	}
}

func mandatoryInputs() []string {
	return []string{"name", "username", "password"}
}

type FakeRunner struct {
	Mock func() error
}
func (f *FakeRunner) Run() error {
	return f.Mock()
}
func (f *FakeRunner) StdinPipe() (io.WriteCloser, error) {
	r := ioutil.NopCloser(&bytes.Buffer{})
	return r, nil
}
