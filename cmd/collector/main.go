//go:build linux
// +build linux

package main

import (
	"flag"
	"os"
	"path"

	pod_collector "github.com/alibaba/kubeskoop/pkg/skoop/collector/podcollector"

	"k8s.io/apimachinery/pkg/util/json"
	log "k8s.io/klog/v2"
)

func main() {
	var dumpPath string
	flag.StringVar(&dumpPath, "dump-path", "/data/collector.json", "Collector result path")
	flag.Parse()
	c, err := pod_collector.NewCollector()
	if err != nil {
		log.Fatalf("error init collector, %v", err)
	}
	result, err := c.DumpNodeInfos()
	if err != nil {
		log.Fatalf("error dump node info: %v", err)
	}
	resultStr, err := json.Marshal(result)
	if err != nil {
		log.Fatalf("error serialize result: %v", err)
	}
	if _, err = os.Stat(path.Dir(dumpPath)); os.IsNotExist(err) {
		_ = os.MkdirAll(path.Dir(dumpPath), 0644)
	}
	err = os.WriteFile(dumpPath, resultStr, 0644)
	if err != nil {
		log.Fatalf("error save result: %v", err)
	}
}
