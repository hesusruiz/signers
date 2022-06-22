package logfilter

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func FilterLogs() {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	// containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	// if err != nil {
	// 	panic(err)
	// }

	// for _, container := range containers {
	// 	fmt.Printf("%s %s\n", container.ID[:10], container.Image)
	// }

	containerID := "0f4ade38f5fc"
	options := types.ContainerLogsOptions{
		ShowStdout: false,
		ShowStderr: true,
	}

	reader, err := cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		log.Fatal(err)
	}

	_, err = io.Copy(os.Stdout, reader)
	if err != nil && err != io.EOF {
		log.Fatal(err)
	}
}
