/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	"github.com/alibaba/kubeskoop/pkg/exporter/proto"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// watchCmd represents the watch command
var (
	watchCmd = &cobra.Command{
		Use:   "watch",
		Short: "A brief description of your command",
		Run: func(cmd *cobra.Command, args []string) {
			endpoint := fmt.Sprintf("%s:%d", endpointAddr, endpointPort)
			slog.Ctx(context.Background()).Info("inspector watch", "endpoint", endpoint)
			watchInspEvents(context.Background(), endpoint)
		},
	}

	endpointPort uint32
	endpointAddr string
)

func init() {
	rootCmd.AddCommand(watchCmd)
	watchCmd.PersistentFlags().Uint32VarP(&endpointPort, "port", "p", 19102, "remote inspector server port")
	watchCmd.PersistentFlags().StringVarP(&endpointAddr, "server", "s", "127.0.0.1", "remote inspector server")
}

func watchInspEvents(ctx context.Context, ep string) {
	conn, err := grpc.Dial(ep, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Ctx(ctx).Warn("grpc dial", "err", err)
		return
	}

	cli := proto.NewInspectorClient(conn)
	stream, err := cli.WatchEvent(ctx, &proto.WatchRequest{})
	if err != nil {
		slog.Ctx(ctx).Warn("stream watch", "err", err, "endpoint", ep)
		return
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			slog.Ctx(ctx).Warn("stream recv", "err", err, "endpoint", ep)
			return
		}

		meta := resp.GetEvent().GetMeta()

		metaStr := fmt.Sprintf("%s/%s node=%s netns=%s ", meta.GetNamespace(), meta.GetPod(), meta.GetNode(), meta.GetNetns())
		slog.Ctx(ctx).Info(resp.GetEvent().GetName(), "meta", metaStr, "event", resp.GetEvent().GetValue())
	}
}
