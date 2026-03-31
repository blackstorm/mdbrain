package main

import (
	"context"
	"log"

	"mdbrain.dev/internal/app"
)

func main() {
	ctx := context.Background()
	projectRoot, err := app.DetectProjectRoot()
	if err != nil {
		log.Fatal(err)
	}
	servers, err := app.Build(ctx, projectRoot)
	if err != nil {
		log.Fatal(err)
	}
	if err := servers.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
