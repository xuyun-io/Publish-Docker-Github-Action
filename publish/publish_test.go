package publish_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"github.com/elgohr/Publish-Docker-Github-Action/publish"
	"github.com/elgohr/Publish-Docker-Github-Action/publish/publishfakes"
	"github.com/sclevine/spec"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPublish(t *testing.T) {
	spec.Run(t, "publishing", func(t *testing.T, when spec.G, it spec.S) {

		var (
			fakeCli    *publishfakes.FakeCli
			testFolder string
		)

		it.Before(func() {
			var err error
			testFolder, err = ioutil.TempDir("", "publish-docker")
			if err != nil {
				log.Fatal(err)
			}
		})

		it("errors when a mandatory input was not set", func() {
			defer unsetMandatoryVariables()

			for i, input := range mandatoryInputs() {
				t.Run(input, func(t *testing.T) {
					for j, input := range mandatoryInputs() {
						if i != j {
							assert.Nil(t, os.Setenv("INPUT_"+strings.ToUpper(input), input))
						}
					}

					err := publish.Publish(&publishfakes.FakeCli{}, testFolder, time.Now())
					expStdout := fmt.Sprintf("Unable to find the %v. Did you set with.%v?", input, input)
					assert.Error(t, err, expStdout)

					for _, input := range mandatoryInputs() {
						assert.Nil(t, os.Unsetenv("INPUT_"+strings.ToUpper(input)))
					}
				})
			}
		})

		when("all mandatory inputs are set", func() {
			it.Before(func() {
				setMandatoryVariables()
				fakeCli = &publishfakes.FakeCli{}
				fakeCli.ImagePushReturns(ioutil.NopCloser(bytes.NewBufferString("")), nil)
			})

			it.After(func() {
				unsetMandatoryVariables()
			})

			it("logs into the default registry and returns the error if any", func() {
				expErr := errors.New("something bad")
				fakeCli.RegistryLoginReturns(registry.AuthenticateOKBody{}, expErr)

				err := publish.Publish(fakeCli, testFolder, time.Now())

				assert.Equal(t, expErr, err)
				context, authConfig := fakeCli.RegistryLoginArgsForCall(0)
				assert.NotNil(t, context)
				assert.Equal(t, "username", authConfig.Username)
				assert.Equal(t, "password", authConfig.Password)
				assert.Equal(t, "registry.hub.docker.com", authConfig.ServerAddress)
			})

			when("a custom registry is configured", func() {
				it.Before(func() {
					if err := os.Setenv("INPUT_REGISTRY", "docker.pkg.github.com"); err != nil {
						log.Fatal(err)
					}
				})
				it.After(func() {
					if err := os.Unsetenv("INPUT_REGISTRY"); err != nil {
						log.Fatal(err)
					}
				})

				it("logs into a custom registry if set", func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
					context, authConfig := fakeCli.RegistryLoginArgsForCall(0)
					assert.NotNil(t, context)
					assert.Equal(t, "username", authConfig.Username)
					assert.Equal(t, "password", authConfig.Password)
					assert.Equal(t, "docker.pkg.github.com", authConfig.ServerAddress)
				})
			})

			when("cache is configured", func() {
				it.Before(func() {
					if err := os.Setenv("INPUT_CACHE", "true"); err != nil {
						log.Fatal(err)
					}
				})
				it.After(func() {
					if err := os.Unsetenv("INPUT_CACHE"); err != nil {
						log.Fatal(err)
					}
				})

				it("pulls the image before building", func() {
					var called bool
					fakeCli.ImagePullCalls(func(ctx context.Context, ref string, options types.ImagePullOptions) (io.ReadCloser, error) {
						called = true
						assert.Equal(t, 0, fakeCli.ImageBuildCallCount())
						return ioutil.NopCloser(bytes.NewBufferString("")), nil
					})

					_ = publish.Publish(fakeCli, testFolder, time.Now())
					assert.True(t, called)
				})

				it("builds the image using the cache", func() {
					fakeCli.ImagePullReturns(ioutil.NopCloser(bytes.NewBufferString("")), nil)

					_ = publish.Publish(fakeCli, testFolder, time.Now())

					ctx, buildCtx, buildOptions := fakeCli.ImageBuildArgsForCall(0)
					assert.NotNil(t, ctx)
					assert.NotNil(t, buildCtx)
					assert.Equal(t, []string{"name"}, buildOptions.CacheFrom)
				})

				it("does not use it for building when it did exist remotely", func() {
					fakeCli.ImagePullReturns(ioutil.NopCloser(bytes.NewBufferString("")), errors.New("not here"))

					_ = publish.Publish(fakeCli, testFolder, time.Now())

					ctx, buildCtx, buildOptions := fakeCli.ImageBuildArgsForCall(0)
					assert.NotNil(t, ctx)
					assert.NotNil(t, buildCtx)
					assert.Equal(t, 0, len(buildOptions.CacheFrom))
				})
			})

			when("cache is not configured", func() {

				it("does not pull the image before building", func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
					assert.Equal(t, 0, fakeCli.ImagePullCallCount())
				})

				it("builds the image without using the cache", func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
					_, _, buildOptions := fakeCli.ImageBuildArgsForCall(0)
					assert.Equal(t, 0, len(buildOptions.CacheFrom))
				})

			})

			it("builds the image and returns the error if any", func() {
				expErr := errors.New("bad")
				fakeCli.ImageBuildReturns(types.ImageBuildResponse{}, expErr)

				err := publish.Publish(fakeCli, testFolder, time.Now())
				assert.Equal(t, expErr, err)
			})

			when("the ref is master", func() {
				it.Before(func() {
					if err := os.Setenv("GITHUB_REF", "refs/heads/master"); err != nil {
						log.Fatal(err)
					}
				})
				it.After(func() {
					if err := os.Unsetenv("GITHUB_REF"); err != nil {
						log.Fatal(err)
					}
				})

				it("tags the image as latest", func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
					_, _, buildOptions := fakeCli.ImageBuildArgsForCall(0)
					assert.Equal(t, []string{"latest"}, buildOptions.Tags)
				})
			})

			when("the ref a branch", func() {
				it.Before(func() {
					if err := os.Setenv("GITHUB_REF", "refs/heads/myBranch"); err != nil {
						log.Fatal(err)
					}
				})
				it.After(func() {
					if err := os.Unsetenv("GITHUB_REF"); err != nil {
						log.Fatal(err)
					}
				})

				it("tags the image as the branch", func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
					_, _, buildOptions := fakeCli.ImageBuildArgsForCall(0)
					assert.Equal(t, []string{"myBranch"}, buildOptions.Tags)
				})
			})

			when("the ref is a tag", func() {
				it.Before(func() {
					if err := os.Setenv("GITHUB_REF", "refs/tags/myRelease"); err != nil {
						log.Fatal(err)
					}
				})
				it.After(func() {
					if err := os.Unsetenv("GITHUB_REF"); err != nil {
						log.Fatal(err)
					}
				})

				it("tags the image as latest", func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
					_, _, buildOptions := fakeCli.ImageBuildArgsForCall(0)
					assert.Equal(t, []string{"latest"}, buildOptions.Tags)
				})
			})

			when("a custom dockerfile is configured", func() {
				it.Before(func() {
					if err := os.Setenv("INPUT_DOCKERFILE", "MyDockerFile"); err != nil {
						log.Fatal(err)
					}
				})
				it.After(func() {
					if err := os.Unsetenv("INPUT_DOCKERFILE"); err != nil {
						log.Fatal(err)
					}
				})

				it("uses it for building", func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
					_, _, buildOptions := fakeCli.ImageBuildArgsForCall(0)
					assert.Equal(t, "MyDockerFile", buildOptions.Dockerfile)
				})

			})

			when("snapshots are configured", func() {
				it.Before(func() {
					if err := os.Setenv("INPUT_SNAPSHOT", "true"); err != nil {
						log.Fatal(err)
					}
					if err := os.Setenv("GITHUB_REF", "refs/heads/master"); err != nil {
						log.Fatal(err)
					}
					if err := os.Setenv("GITHUB_SHA", "1dbfb40621d5d4a13d51d97b3f52732fda0432ad"); err != nil {
						log.Fatal(err)
					}
				})
				it.After(func() {
					if err := os.Unsetenv("INPUT_SNAPSHOT"); err != nil {
						log.Fatal(err)
					}
					if err := os.Unsetenv("GITHUB_REF"); err != nil {
						log.Fatal(err)
					}
					if err := os.Unsetenv("GITHUB_SHA"); err != nil {
						log.Fatal(err)
					}
				})

				it("builds an additional tag", func() {
					now := time.Now()
					_ = publish.Publish(fakeCli, testFolder, now)
					_, _, buildOptions := fakeCli.ImageBuildArgsForCall(0)
					snapshot := now.Format("20060102150405") + "1dbfb4"
					assert.Equal(t, []string{"latest", snapshot}, buildOptions.Tags)
				})

				it("pushes both tags", func() {
					now := time.Now()

					_ = publish.Publish(fakeCli, testFolder, now)

					assert.Equal(t, 2, fakeCli.ImagePushCallCount())
					_, firstRef, _ := fakeCli.ImagePushArgsForCall(0)
					assert.Equal(t, "name:latest", firstRef)
					_, secondRef, _ := fakeCli.ImagePushArgsForCall(1)
					snapshot := now.Format("20060102150405") + "1dbfb4"
					assert.Equal(t, "name:"+snapshot, secondRef)
				})
			})

			it("pushes the image and returns the error if any", func() {
				if err := os.Setenv("INPUT_NAME", "myImageName"); err != nil {
					log.Fatal(err)
				}
				if err := os.Setenv("GITHUB_REF", "refs/heads/master"); err != nil {
					log.Fatal(err)
				}
				defer os.Unsetenv("GITHUB_REF")

				expErr := errors.New("bad")
				fakeCli.ImagePushReturns(ioutil.NopCloser(bytes.NewBufferString("")), expErr)

				err := publish.Publish(fakeCli, testFolder, time.Now())
				assert.Equal(t, expErr, err)
				ctx, ref, options := fakeCli.ImagePushArgsForCall(0)
				assert.NotNil(t, ctx)
				assert.NotNil(t, options)
				assert.Equal(t, "myImageName:latest", ref)
			})
		})
	})
}

func setMandatoryVariables() {
	for _, input := range mandatoryInputs() {
		if err := os.Setenv("INPUT_"+strings.ToUpper(input), input); err != nil {
			log.Fatal(err)
		}
	}
}

func unsetMandatoryVariables() {
	for _, input := range mandatoryInputs() {
		if err := os.Unsetenv("INPUT_" + strings.ToUpper(input)); err != nil {
			log.Fatal(err)
		}
	}
}

func mandatoryInputs() []string {
	return []string{"name", "username", "password"}
}
