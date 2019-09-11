package publish

import (
	"context"
	"errors"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"io"
	"os"
	"strings"
	"time"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . Cli
type Cli interface {
	RegistryLogin(ctx context.Context, auth types.AuthConfig) (registry.AuthenticateOKBody, error)
	ImagePull(ctx context.Context, ref string, options types.ImagePullOptions) (io.ReadCloser, error)
	ImageBuild(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error)
	ImagePush(ctx context.Context, ref string, options types.ImagePushOptions) (io.ReadCloser, error)
}

func Publish(cli Cli, path string, buildTime time.Time) error {
	name := os.Getenv("INPUT_NAME")
	username := os.Getenv("INPUT_USERNAME")
	password := os.Getenv("INPUT_PASSWORD")
	e := sanitizeInput(name, username, password)
	if e != nil {
		return e
	}

	ctx := context.Background()
	config := types.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: readServerAddress(),
	}
	if _, err := cli.RegistryLogin(ctx, config); err != nil {
		return err
	}

	buildOptions := types.ImageBuildOptions{}
	if os.Getenv("INPUT_CACHE") != "" {
		if r, err := cli.ImagePull(ctx, name, types.ImagePullOptions{}); err == nil {
			r.Close()
			buildOptions.CacheFrom = []string{name}
		}
	}

	useTags(&buildOptions)
	createSnapshotTag(buildTime, &buildOptions)
	useCustomDockerFile(&buildOptions)

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	if _, err := cli.ImageBuild(ctx, file, buildOptions); err != nil {
		return err
	}

	for _, tag := range buildOptions.Tags {
		imgRef := name + ":" + tag
		r, err := cli.ImagePush(ctx, imgRef, types.ImagePushOptions{})
		if err != nil {
			return err
		}
		r.Close()
	}

	return nil
}

func createSnapshotTag(buildTime time.Time, buildOptions *types.ImageBuildOptions) {
	if os.Getenv("INPUT_SNAPSHOT") != "" {
		snapshotTag := buildTime.Format("20060102150405") + os.Getenv("GITHUB_SHA")[:6]
		buildOptions.Tags = append(buildOptions.Tags, snapshotTag)
	}
}

func useCustomDockerFile(buildOptions *types.ImageBuildOptions) {
	customDockerFile := os.Getenv("INPUT_DOCKERFILE")
	if customDockerFile != "" {
		buildOptions.Dockerfile = customDockerFile
	}
}

func useTags(buildOptions *types.ImageBuildOptions) {
	ref := os.Getenv("GITHUB_REF")
	if ref == "refs/heads/master" {
		buildOptions.Tags = []string{"latest"}
	} else if strings.HasPrefix(ref, "refs/tags") {
		buildOptions.Tags = []string{"latest"}
	} else {
		buildOptions.Tags = []string{strings.TrimPrefix(ref, "refs/heads/")}
	}
}

func readServerAddress() string {
	customRegistry := os.Getenv("INPUT_REGISTRY")
	if customRegistry != "" {
		return customRegistry
	}
	return "registry.hub.docker.com"
}

func sanitizeInput(name string, username string, password string) error {
	if name == "" {
		return errors.New("Unable to find the name. Did you set with.name?")
	} else if username == "" {
		return errors.New("Unable to find the username. Did you set with.username?")
	} else if password == "" {
		return errors.New("Unable to find the password. Did you set with.password?")
	}
	return nil
}
