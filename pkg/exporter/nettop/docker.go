package nettop

import (
	"context"
	"encoding/json"
	"fmt"
	io "io"
	"net"
	"net/http"

	log "github.com/sirupsen/logrus"
)

var (
	dockerhttpc *http.Client
)

type slimDocker struct {
	ID    string          `json:"Id,omitempty"`
	State slimDockerState `json:"State"`
}

type slimDockerState struct {
	Status string `json:"Status"`
	Pid    int    `json:"Pid"`
}

func getPidForContainerBySock(id string) (int, error) {
	// logger.Infof("start get pid of %s", id)
	dockersock := "/var/run/docker.sock"
	if dockerhttpc == nil {
		dockerhttpc = &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", dockersock)
				},
			},
		}
	}

	url := fmt.Sprintf("http://localhost/containers/%s/json", id)
	response, err := dockerhttpc.Get(url)
	if err != nil {
		log.Errorf("failed get docker response, err: %v", err)
		return 0, err
	}

	b, err := io.ReadAll(response.Body)
	if err != nil {
		log.Errorf("failed get docker response, err: %v", err)
		return 0, err
	}

	sd := &slimDocker{}
	err = json.Unmarshal(b, &sd)
	if err != nil {
		log.Errorf("failed get docker response, err: %v", err)
		return 0, err
	}
	log.Infof("finish get pid, sandbox: %s, pid: %d", id, sd.State.Pid)
	return sd.State.Pid, nil
}
