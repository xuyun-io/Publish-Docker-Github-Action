package main

import (
	"github.com/docker/docker/client"
	"github.com/elgohr/Publish-Docker-Github-Action/publish"
	"log"
	"path/filepath"
	"time"
)

func main() {
	cli, err := client.NewEnvClient()
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()
	currentFolder := "." + string(filepath.Separator)
	err = publish.Publish(cli, currentFolder, time.Now())
	if err != nil {
		log.Fatal(err)
	}
}


