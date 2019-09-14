package publish_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"github.com/elgohr/Publish-Docker-Github-Action/publish"
	"github.com/elgohr/Publish-Docker-Github-Action/publish/publishfakes"
	"github.com/kami-zh/go-capturer"
	"github.com/sclevine/spec"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
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
			if err := ioutil.WriteFile(filepath.Join(testFolder, "Dockerfile"), []byte("FROM scratch"), 777); err != nil {
				log.Fatal(err)
			}
		})

		it.Before(func() {
			if err := os.Setenv("GITHUB_REF", "refs/heads/master"); err != nil {
				log.Fatal(err)
			}
		})

		it.Before(func() {
			fakeCli = &publishfakes.FakeCli{}
			fakeCli.ImageBuildReturns(types.ImageBuildResponse{
				Body:   ioutil.NopCloser(bytes.NewBufferString("")),
				OSType: "linux",
			}, nil)
			fakeCli.ImagePushReturns(ioutil.NopCloser(bytes.NewBufferString("")), nil)
			fakeCli.ImagePullReturns(ioutil.NopCloser(bytes.NewBufferString("")), nil)
		})

		it("errors when a mandatory input was not set", func() {
			defer unsetMandatoryVariables()
			mandatoryInputs := []string{"INPUT_NAME", "INPUT_USERNAME", "INPUT_PASSWORD"}

			for i, input := range mandatoryInputs {
				t.Run(input, func(t *testing.T) {
					for j, input := range mandatoryInputs {
						if i != j {
							assert.Nil(t, os.Setenv("INPUT_"+strings.ToUpper(input), input))
						}
					}

					err := publish.Publish(&publishfakes.FakeCli{}, testFolder, time.Now())
					expStdout := fmt.Sprintf("Unable to find the %v. Did you set with.%v?", input, input)
					assert.Error(t, err, expStdout)

					for _, input := range mandatoryInputs {
						assert.Nil(t, os.Unsetenv("INPUT_"+strings.ToUpper(input)))
					}
				})
			}
		})

		when("all mandatory inputs are set", func() {
			it.Before(func() {
				setMandatoryVariables()
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
				assert.Equal(t, "USERNAME", authConfig.Username)
				assert.Equal(t, "PASSWORD", authConfig.Password)
				assert.Equal(t, "docker.io", authConfig.ServerAddress)
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
					assert.Equal(t, "USERNAME", authConfig.Username)
					assert.Equal(t, "PASSWORD", authConfig.Password)
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

				it("pulls the image", func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
					ctx, ref, options := fakeCli.ImagePullArgsForCall(0)
					assert.NotNil(t, ctx)
					assert.Equal(t, "docker.io/my/testimage:latest", ref)
					assert.Equal(t, "eyJ1c2VybmFtZSI6IlVTRVJOQU1FIiwicGFzc3dvcmQiOiJ"+
						"QQVNTV09SRCIsInNlcnZlcmFkZHJlc3MiOiJkb2NrZXIuaW8ifQ==", options.RegistryAuth)
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
					err := publish.Publish(fakeCli, testFolder, time.Now())
					assert.Nil(t, err)

					ctx, buildCtx, buildOptions := fakeCli.ImageBuildArgsForCall(0)
					assert.NotNil(t, ctx)
					assert.NotNil(t, buildCtx)
					assert.Equal(t, []string{"docker.io/my/testimage:latest"}, buildOptions.CacheFrom)
				})

				it("does not use it for building when it did not exist remotely", func() {
					fakeCli.ImagePullReturns(nil, errors.New("not here"))

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
				ctx, buildCtx, options := fakeCli.ImageBuildArgsForCall(0)
				assert.NotNil(t, ctx)
				tr := tar.NewReader(buildCtx)
				var files = make(map[string]string)
				for {
					hdr, err := tr.Next()
					if err == io.EOF {
						break // End of archive
					}
					if err != nil {
						log.Fatal(err)
					}
					content, err := ioutil.ReadAll(tr)
					if err != nil {
						log.Fatal(err)
					}
					files[hdr.Name] = string(content)
				}
				assert.Equal(t, "FROM scratch", files["Dockerfile"])
				assert.Equal(t, []string{"docker.io/my/testimage:latest"}, options.Tags)
			})

			it("logs the output of the build", func() {
				fakeCli.ImageBuildReturns(types.ImageBuildResponse{
					Body: ioutil.NopCloser(bytes.NewBufferString(`{"stream":"Step 1/2 : FROM ubuntu"}
{"stream":"\n"}
{"stream":" ---\u003e a2a15febcdf3\n"}
{"stream":"Step 2/2 : RUN echo \"hello\""}
{"stream":"\n"}
{"stream":" ---\u003e Using cache\n"}
{"stream":" ---\u003e 70022947d651\n"}
{"stream":"Successfully built 70022947d651\n"}
{"stream":"Successfully tagged lgohr/testimage:latest\n"}`)),
					OSType: "linux",
				}, nil)

				stdOut := capturer.CaptureStdout(func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
				})
				expBuildOutput := "Step 1/2 : FROM ubuntu\n\n\n ---> a2a15febcdf3\n\n" +
					"Step 2/2 : RUN echo \"hello\"\n\n\n ---> Using cache\n\n ---> 70022947d651\n\n" +
					"Successfully built 70022947d651\n\nSuccessfully tagged lgohr/testimage:latest\n\n"
				assert.True(t, strings.HasPrefix(stdOut, expBuildOutput), stdOut)
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
					assert.Equal(t, []string{"docker.io/my/testimage:latest"}, buildOptions.Tags)
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
					assert.Equal(t, []string{"docker.io/my/testimage:myBranch"}, buildOptions.Tags)
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
					assert.Equal(t, []string{"docker.io/my/testimage:latest"}, buildOptions.Tags)
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
					tags := []string{"docker.io/my/testimage:latest", "docker.io/my/testimage:" + snapshot}
					assert.Equal(t, buildOptions.Tags, tags)
				})

				it("pushes both tags", func() {
					now := time.Now()

					_ = publish.Publish(fakeCli, testFolder, now)

					assert.Equal(t, 2, fakeCli.ImagePushCallCount())
					_, firstRef, _ := fakeCli.ImagePushArgsForCall(0)
					assert.Equal(t, "docker.io/my/testimage:latest", firstRef)
					_, secondRef, _ := fakeCli.ImagePushArgsForCall(1)
					snapshot := now.Format("20060102150405") + "1dbfb4"
					assert.Equal(t, "docker.io/my/testimage:"+snapshot, secondRef)
				})
			})

			it("pushes the image and returns the error if any", func() {
				expErr := errors.New("bad")
				fakeCli.ImagePushReturns(ioutil.NopCloser(bytes.NewBufferString("")), expErr)

				err := publish.Publish(fakeCli, testFolder, time.Now())
				assert.Equal(t, expErr, err)
				ctx, ref, options := fakeCli.ImagePushArgsForCall(0)
				assert.NotNil(t, ctx)
				assert.NotNil(t, options)
				assert.Equal(t, "docker.io/my/testimage:latest", ref)
			})

			it("logs the output of the push", func() {
				fakeCli.ImagePushReturns(ioutil.NopCloser(bytes.NewBufferString(`{"status":"The push refers to repository [docker.io/lgohr/testimage]"}
{"status":"Preparing","progressDetail":{},"id":"122be11ab4a2"}
{"status":"Preparing","progressDetail":{},"id":"7beb13bce073"}
{"status":"Preparing","progressDetail":{},"id":"f7eae43028b3"}
{"status":"Preparing","progressDetail":{},"id":"6cebf3abed5f"}
{"status":"Layer already exists","progressDetail":{},"id":"6cebf3abed5f"}
{"status":"Layer already exists","progressDetail":{},"id":"f7eae43028b3"}
{"status":"Layer already exists","progressDetail":{},"id":"7beb13bce073"}
{"status":"Layer already exists","progressDetail":{},"id":"122be11ab4a2"}
{"status":"latest: digest: sha256:4dcf2a2544360335a53ab9188925b3e819a099d5817c87d107cdffae6c7ea028 size: 1152"}
{"progressDetail":{},"aux":{"Tag":"latest","Digest":"sha256:4dcf2a2544360335a53ab9188925b3e819a099d5817c87d107cdffae6c7ea028","Size":1152}}`)), nil)

				stdOut := capturer.CaptureStdout(func() {
					_ = publish.Publish(fakeCli, testFolder, time.Now())
				})
				expBuildOutput := `The push refers to repository [docker.io/lgohr/testimage]
Preparing
Preparing
Preparing
Preparing
Layer already exists
Layer already exists
Layer already exists
Layer already exists
latest: digest: sha256:4dcf2a2544360335a53ab9188925b3e819a099d5817c87d107cdffae6c7ea028 size: 1152

`
				assert.Equal(t, expBuildOutput, stdOut)
			})
		})
	})
}

func setMandatoryVariables() {
	if err := os.Setenv("INPUT_NAME", "my/testimage"); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("INPUT_USERNAME", "USERNAME"); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("INPUT_PASSWORD", "PASSWORD"); err != nil {
		log.Fatal(err)
	}
}

func unsetMandatoryVariables() {
	if err := os.Unsetenv("INPUT_NAME"); err != nil {
		log.Fatal(err)
	}
	if err := os.Unsetenv("INPUT_USERNAME"); err != nil {
		log.Fatal(err)
	}
	if err := os.Unsetenv("INPUT_PASSWORD"); err != nil {
		log.Fatal(err)
	}
}
