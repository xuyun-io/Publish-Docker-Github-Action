package main

import(
	"github.com/elgohr/Publish-Docker-Github-Action/publish"
	"os"
)

func main() {
	os.Exit(publish.Run())
}


