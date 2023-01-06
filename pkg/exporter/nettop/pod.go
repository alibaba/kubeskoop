package nettop

import (
	"encoding/json"
	"errors"

	internalapi "k8s.io/cri-api/pkg/apis"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type NamespacesSpec struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type LinuxSpec struct {
	NamespaceSpec []NamespacesSpec `json:"namespaces"`
}

type RuntimeSpec struct {
	Linux LinuxSpec `json:"linux"`
}

type InfoSpec struct {
	Pid         int         `json:"pid"`
	RuntimeSpec RuntimeSpec `json:"runtimeSpec"`
}

func getPodMetas(client internalapi.RuntimeService) (map[string]podMeta, error) {
	if client == nil {
		return nil, errors.New("not in cloudnative environment")
	}
	// only list live pods
	filter := runtimeapi.PodSandboxFilter{
		State: &runtimeapi.PodSandboxStateValue{
			State: runtimeapi.PodSandboxState_SANDBOX_READY,
		},
	}
	listresponse, err := client.ListPodSandbox(&filter)
	if err != nil {
		return nil, err
	}
	resMap := make(map[string]podMeta)
	for _, sandbox := range listresponse {
		status, err := client.PodSandboxStatus(sandbox.GetId(), true)
		if err != nil {
			logger.Debug("get pod sanbox %s status failed with %s", sandbox.GetId(), err)
			continue
		}
		pm := podMeta{
			sandbox:   sandbox.GetId(),
			name:      status.GetStatus().GetMetadata().GetName(),
			namespace: status.GetStatus().GetMetadata().GetNamespace(),
			ip:        status.GetStatus().GetNetwork().GetIp(),
		}

		if v, ok := status.GetStatus().GetLabels()["app"]; ok {
			pm.app = v
		}

		// get process pid
		info := status.GetInfo()["info"]
		if info != "" {
			infospec := InfoSpec{}
			err := json.Unmarshal([]byte(info), &infospec)
			if err != nil {
				logger.Warn("parse info spec %s failed with %s", pm.name, err)
				continue
			}
			pm.pid = infospec.Pid
			if infospec.RuntimeSpec.Linux.NamespaceSpec != nil && len(infospec.RuntimeSpec.Linux.NamespaceSpec) > 0 {
				for _, ns := range infospec.RuntimeSpec.Linux.NamespaceSpec {
					if ns.Type == "network" {
						pm.nspath = ns.Path
					}
				}
			}
		}

		resMap[sandbox.GetId()] = pm

	}
	return resMap, nil
}
