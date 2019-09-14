package publish

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/registry"
	"github.com/mholt/archiver"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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
	if err := sanitizeInput(name, username, password); err != nil {
		return err
	}

	ctx := context.Background()

	authConfig := types.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: readServerAddress(),
	}
	_, err := cli.RegistryLogin(ctx, authConfig)
	if err != nil {
		return err
	}

	canonicalName := readServerAddress() + "/" + name
	canonicalTaggedName := canonicalName + ":" + translateRefToTag(os.Getenv("GITHUB_REF"))

	buildOptions := types.ImageBuildOptions{}
	if os.Getenv("INPUT_CACHE") != "" {
		if pullResponse, err := cli.ImagePull(ctx, canonicalTaggedName, types.ImagePullOptions{
			RegistryAuth: authString(authConfig),
		}); err == nil {
			defer pullResponse.Close()
			logPull(pullResponse)
			buildOptions.CacheFrom = []string{canonicalTaggedName}
		}
	}

	createLatestTag(&buildOptions, canonicalTaggedName)
	createSnapshotTag(&buildOptions, buildTime, canonicalName)
	useCustomDockerFile(&buildOptions)

	compressedInput, err := compressInput(path)
	if err != nil {
		return err
	}

	imageResponse, err := cli.ImageBuild(ctx, compressedInput, buildOptions)
	if err != nil {
		return err
	}
	defer imageResponse.Body.Close()
	logBuild(imageResponse)

	for _, ref := range buildOptions.Tags {
		pushResponse, err := cli.ImagePush(ctx, ref, types.ImagePushOptions{
			RegistryAuth: authString(authConfig),
		})
		if err != nil {
			return err
		}
		defer pushResponse.Close()
		logPush(pushResponse)
	}

	return nil
}

func logPull(pullResponse io.ReadCloser) {
	scanner := bufio.NewScanner(pullResponse)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
}

func logBuild(imageResponse types.ImageBuildResponse) {
	scanner := bufio.NewScanner(imageResponse.Body)
	for scanner.Scan() {
		var buildLogEntries BuildLog
		if err := json.Unmarshal(scanner.Bytes(), &buildLogEntries); err == nil {
			fmt.Println(buildLogEntries.Entry)
		}
	}
}

func logPush(pushResponse io.ReadCloser) {
	scanner := bufio.NewScanner(pushResponse)
	for scanner.Scan() {
		var pushLogEntry PushLog
		if err := json.Unmarshal(scanner.Bytes(), &pushLogEntry); err == nil {
			fmt.Println(pushLogEntry.Entry)
		}
	}
}

func createLatestTag(buildOptions *types.ImageBuildOptions, canonicalTaggedName string) {
	buildOptions.Tags = []string{canonicalTaggedName}
}

func compressInput(path string) (*os.File, error) {
	var files []string
	if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	}); err != nil {
		return nil, err
	}
	tmpDir, _ := ioutil.TempDir("", "*")
	filePath := filepath.Join(tmpDir, "test.tar")
	if err := archiver.Archive(files, filePath); err != nil {
		return nil, err
	}
	compressedContext, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	return compressedContext, err
}

func authString(authConfig types.AuthConfig) string {
	encodedJSON, _ := json.Marshal(authConfig)
	return base64.URLEncoding.EncodeToString(encodedJSON)
}

func createSnapshotTag(buildOptions *types.ImageBuildOptions, buildTime time.Time, canonicalName string) {
	if os.Getenv("INPUT_SNAPSHOT") != "" {
		snapshotTag := buildTime.Format("20060102150405") + os.Getenv("GITHUB_SHA")[:6]
		buildOptions.Tags = append(buildOptions.Tags, canonicalName+":"+snapshotTag)
	}
}

func useCustomDockerFile(buildOptions *types.ImageBuildOptions) {
	customDockerFile := os.Getenv("INPUT_DOCKERFILE")
	if customDockerFile != "" {
		buildOptions.Dockerfile = customDockerFile
	}
}

func translateRefToTag(ref string) (tag string) {
	if ref == "refs/heads/master" {
		return "latest"
	} else if strings.HasPrefix(ref, "refs/tags") {
		return "latest"
	}
	return strings.TrimPrefix(ref, "refs/heads/")
}

func readServerAddress() string {
	customRegistry := os.Getenv("INPUT_REGISTRY")
	if customRegistry != "" {
		return customRegistry
	}
	return "docker.io"
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

type BuildLog struct {
	Entry string `json:"stream"`
}

type PushLog struct {
	Entry string `json:"status"`
}
