package nettop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

var (
	dockerhttpc *http.Client
	dockerInfo  *slimDockerInfo
)

const dockersock = "/var/run/docker.sock"

type slimDocker struct {
	State      slimDockerState      `json:"State"`
	HostConfig slimDockerHostConfig `json:"HostConfig"`
}

type slimDockerState struct {
	Status string `json:"Status"`
	Pid    int    `json:"Pid"`
}

type slimDockerHostConfig struct {
	CgroupParent string `json:"CgroupParent"`
}

type slimDockerInfo struct {
	CgroupDriver string `json:"CgroupDriver"`
}

func initializeDockerClient() {
	if dockerhttpc != nil {
		return
	}
	dockerhttpc = &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", dockersock)
			},
		},
	}

}

func initializeDockerInfo() error {
	info, err := dockerHTTPRequest("/info")
	if err != nil {
		return err
	}
	dockerInfo = &slimDockerInfo{}
	if err := json.Unmarshal(info, dockerInfo); err != nil {
		return fmt.Errorf("failed unmarshal %s to slimDockerInfo: %w", string(info), err)
	}
	return nil
}

func dockerHTTPRequest(path string) ([]byte, error) {

	path = strings.TrimPrefix(path, "/")

	url := fmt.Sprintf("http://localhost/%s", path)

	resp, err := dockerhttpc.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed request docker %s: %w", url, err)
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed read docker response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed request docker %s, status code: %d, body: %s", url, resp.StatusCode, string(data))
	}

	return data, nil
}

func getSandboxInfoSpecForDocker(id string) (*sandboxInfoSpec, error) {
	if dockerhttpc == nil {
		initializeDockerClient()
	}

	if dockerInfo == nil {
		if err := initializeDockerInfo(); err != nil {
			return nil, fmt.Errorf("failed get docker info: %w", err)
		}
	}

	b, err := dockerHTTPRequest(fmt.Sprintf("/containers/%s/json", id))
	if err != nil {
		return nil, fmt.Errorf("failed read container detail for %s from docker docker: %w", id, err)
	}

	docker := &slimDocker{}
	if err := json.Unmarshal(b, docker); err != nil {
		return nil, fmt.Errorf("failed unmarsh docker response for %s to slimDocker: %w", id, err)
	}

	return buildSandboxInfoSpecForDocker(docker)

}

func adjustCgroupParent(rawCgroupParent string) string {
	if dockerInfo.CgroupDriver == "cgroupfs" {
		return rawCgroupParent
	}

	arr := strings.Split(rawCgroupParent, "-")

	if len(arr) == 2 {
		//guaranteed pod, cgroup parent: kubepods-podfd9ea419_d65a_454e_9697_f7312cf47af7.slice
		return fmt.Sprintf("/%[1]s.slice/%[1]s-%[2]s", arr[0], arr[1])
	} else if len(arr) == 3 {
		//besteffort and burstable pod, cgroup parent: kubepods-burstable-podf4c0cdd8e0920b18d85d50904cb0f13d.slice
		return fmt.Sprintf("/%[1]s.slice/%[1]s-%[2]s.slice/%[1]s-%[2]s-%[3]s", arr[0], arr[1], arr[2])
	}

	log.Errorf("invalid cgroup parent path: %s", rawCgroupParent)
	return ""
}

func buildSandboxInfoSpecForDocker(docker *slimDocker) (*sandboxInfoSpec, error) {
	cgroupParent := adjustCgroupParent(docker.HostConfig.CgroupParent)
	return &sandboxInfoSpec{
		Pid: docker.State.Pid,
		Config: Config{
			Linux: Linux{
				CgroupParent: cgroupParent,
			},
		},
	}, nil
}
