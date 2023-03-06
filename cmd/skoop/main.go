package main

import (
	"github.com/alibaba/kubeskoop/pkg/skoop/cmd"
	"k8s.io/klog/v2"
)

func main() {
	c := cmd.NewSkoopCmd()
	if err := c.Execute(); err != nil {
		klog.Fatalf("error on skoop: %s", err)
	}
}
