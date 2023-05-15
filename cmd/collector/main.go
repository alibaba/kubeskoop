//go:build linux
// +build linux

package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"runtime"

	pod_collector "github.com/alibaba/kubeskoop/pkg/skoop/collector/podcollector"

	"k8s.io/apimachinery/pkg/util/json"
	log "k8s.io/klog/v2"
)

func main() {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()

	if os.Args[0] == "nsenter" {
		output, err := pod_collector.NSExec(os.Args)
		if err != nil {
			os.Stderr.WriteString(fmt.Sprintf("error: %v\n", err))
			os.Stderr.Sync()
			os.Exit(1)
		}
		fmt.Print(output)
		return
	}
	var (
		dumpPath, podNamespace, podName, runtimeEndpoint string
	)
	flag.StringVar(&dumpPath, "dump-path", "/data/collector.json", "Collector result path")
	flag.StringVar(&podNamespace, "namespace", "", "pod namespace to collect")
	flag.StringVar(&podName, "name", "", "pod name to collect, 'host' as host network namespace")
	flag.StringVar(&runtimeEndpoint, "runtime-endpoint", "", "runtime socket addr to resolve pod info")
	flag.Parse()
	c, err := pod_collector.NewCollector(podNamespace, podName, runtimeEndpoint)
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
