package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/alibaba/kubeskoop/pkg/skoop/context"
	"github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/provider"
	"github.com/alibaba/kubeskoop/pkg/skoop/ui"

	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	err := loadSkoopConfig()
	if err != nil {
		klog.Fatalf("error load skoop config: %v", err)
	}
	prvd, err := provider.GetProvider(context.SkoopContext.ClusterConfig().CloudProvider)
	if err != nil {
		klog.Fatalf("error get service provider: %v", err)
	}

	network, err := prvd.CreateNetwork(context.SkoopContext)
	if err != nil {
		klog.Fatalf("error create network: %v", err)
	}

	globalSuspicion, packetPath, err := network.Diagnose(context.SkoopContext, context.SkoopContext.TaskConfig().SourceEndpoint, context.SkoopContext.TaskConfig().DstEndpoint)
	if err != nil {
		//TODO error的情况下，如果已经有部分结果，应该将结果输出
		klog.Fatalf("diagnose error: %v", err)
	}

	if context.SkoopContext.UIConfig().HTTP {
		klog.Fatalf("http server exited: %s", serveWebUI(packetPath))
	}

	if context.SkoopContext.UIConfig().Format != "" {
		switch context.SkoopContext.UIConfig().Format {
		case "svg", "dot":
			err = saveGraphFile(packetPath)
			if err != nil {
				klog.Fatalf("save graph file error: %v", err)
			}
		case "json":
			err = saveJSONFile(packetPath)
			if err != nil {
				klog.Fatalf("save json file error: %v", err)
			}
		}

	} else {
		fmt.Printf("Packet path:\n%+v\n", packetPath.Paths())
	}

	suspicions := formatSuspicions(globalSuspicion, packetPath)
	if len(suspicions) != 0 {
		fmt.Printf("\n%s", suspicions)
	}
}

func saveGraphFile(p *model.PacketPath) error {
	g, err := ui.NewGraphviz(p)
	if err != nil {
		return err
	}

	var data []byte
	switch context.SkoopContext.UIConfig().Format {
	case "svg":
		data, err = g.ToSvg()
		if err != nil {
			return err
		}
	case "dot":
		data, err = g.ToDot()
		if err != nil {
			return err
		}
	}

	fileName := context.SkoopContext.UIConfig().Output
	if fileName == "" {
		fileName = fmt.Sprintf("output.%s", context.SkoopContext.UIConfig().Format)
	}

	if fileName == "-" {
		// write to stdout
		_, err = os.Stdout.Write(data)
		if err != nil {
			return err
		}
	} else {
		err = os.WriteFile(fileName, data, 0666)
		if err != nil {
			return err
		}
		klog.V(0).Infof("File has been saved to %s.", fileName)
	}

	return nil
}

func saveJSONFile(p *model.PacketPath) error {
	formatter := ui.NewJSONFormatter(p)
	data, err := formatter.ToJSON()
	if err != nil {
		return err
	}

	fileName := context.SkoopContext.UIConfig().Output
	if fileName == "" {
		fileName = "output.json"
	}

	if fileName == "-" {
		// write to stdout
		_, err = os.Stdout.Write(data)
		if err != nil {
			return err
		}
	} else {
		err = os.WriteFile(fileName, data, 0666)
		if err != nil {
			return err
		}
		klog.V(0).Infof("File has been saved to %s.", fileName)
	}

	return err
}

func serveWebUI(p *model.PacketPath) error {
	web, err := ui.NewWebUI(context.SkoopContext, p, context.SkoopContext.UIConfig().HTTPAddress)
	if err != nil {
		return err
	}

	return web.Serve()
}

func formatSuspicions(globalSuspicion []model.Suspicion, packetPath *model.PacketPath) string {
	var builder strings.Builder

	if len(globalSuspicion) > 0 {
		builder.WriteString("Suspicions on cluster:\n")
		for _, s := range globalSuspicion {
			builder.WriteString(fmt.Sprintf("[%s] %s\n", s.Level, s.Message))
		}
	}

	nodes := packetPath.Nodes()
	for _, n := range nodes {
		suspicions := n.GetSuspicions()
		if len(suspicions) > 0 {
			builder.WriteString(fmt.Sprintf("Suspicions on node %q\n", n.GetID()))
			for _, s := range suspicions {
				builder.WriteString(fmt.Sprintf("[%s] %s\n", s.Level, s.Message))
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func loadSkoopConfig() error {
	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	context.SkoopContext.BindFlags(fs)
	if err := fs.Parse(os.Args); err != nil {
		return fmt.Errorf("error parse args: %v", err)
	}
	if err := context.SkoopContext.Validate(); err != nil {
		return fmt.Errorf("error validate args: %v", err)
	}
	if err := context.SkoopContext.BuildCluster(); err != nil {
		return fmt.Errorf("error build cluster config: %v", err)
	}
	if err := context.SkoopContext.BuildTask(); err != nil {
		return fmt.Errorf("error build task config: %v", err)
	}
	return nil
}
