package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/google/trillian"
	"github.com/google/trillian/client/rpcflags"
	"google.golang.org/grpc"
)

func main() {
	var adminServerAddress string
	var showDeleted bool

	flag.StringVar(&adminServerAddress, "admin_server", "", "The trillian admin server address")
	flag.BoolVar(&showDeleted, "show_deleted", false, "Whether to show deleted logs as well")
	flag.Parse()

	if adminServerAddress == "" {
		log.Fatalf("admin_server flag must be set")
	}

	dialOptions, err := rpcflags.NewClientDialOptionsFromFlags()
	if err != nil {
		log.Fatalf("error during dial options creation %v", err)
	}

	conn, err := grpc.Dial(adminServerAddress, dialOptions...)
	if err != nil {
		log.Fatalf("error connecting to '%s': %v", adminServerAddress, err)
	}

	adminClient := trillian.NewTrillianAdminClient(conn)
	response, err := adminClient.ListTrees(context.Background(), &trillian.ListTreesRequest{
		ShowDeleted: showDeleted,
	})

	if err != nil {
		log.Fatalf("error while listing logs: %v", err)
	}

	for _, tree := range response.Tree {
		deletedTime := tree.DeleteTime.AsTime().Format("2006-01-02 15:04:05-07")
		if !tree.Deleted {
			deletedTime = "-"
		}

		fmt.Printf("%s (%d), type: %s, state: %s, creation time: %s, deletion time: %s\n", tree.DisplayName, tree.TreeId, tree.TreeType.String(), tree.TreeState.String(), tree.CreateTime.AsTime().Format("2006-01-02 15:04:05-07"), deletedTime)
	}
}
