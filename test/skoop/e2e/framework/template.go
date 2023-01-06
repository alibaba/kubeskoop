package framework

import (
	"bytes"
	"fmt"
	"text/template"
)

type commandNodeInfo struct {
	IP      string
	PodCIDR string
	Extra   map[string]string
}

type commandPodInfo struct {
	IP    string
	Extra map[string]string
}

type commandInfo struct {
	Node map[string]commandNodeInfo
	Pod  map[string]commandPodInfo
}

func (f *Framework) renderCommand(cmd string) (string, error) {
	t, err := template.New("command").Parse(cmd)
	if err != nil {
		return "", err
	}

	buffer := &bytes.Buffer{}
	err = t.Execute(buffer, &f.commandInfo)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}

func (f *Framework) setPodCommandInfo() error {
	f.commandInfo.Pod = map[string]commandPodInfo{}
	for _, n := range f.spec.NodeSpecs {
		for _, p := range n.PodSpecs {
			if p.pod == nil {
				return fmt.Errorf("pod %s actual pod is nil", p.ID)
			}
			f.commandInfo.Pod[p.ID] = commandPodInfo{
				IP:    p.pod.Status.PodIP,
				Extra: p.ExtraInfo,
			}
		}
	}
	return nil
}

func (f *Framework) setNodeCommandInfo() error {
	f.commandInfo.Node = map[string]commandNodeInfo{}
	for _, n := range f.spec.NodeSpecs {
		if n.node == nil {
			return fmt.Errorf("node %s actual node is nil", n.ID)
		}
		f.commandInfo.Node[n.ID] = commandNodeInfo{
			IP:      n.node.Status.Addresses[0].Address,
			PodCIDR: n.node.Spec.PodCIDR,
			Extra:   n.ExtraInfo,
		}
	}
	return nil
}
