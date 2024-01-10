package nettop

// from containerd sandbox status
//type NamespacesSpec struct {
//	Type string `json:"type"`
//	Path string `json:"path"`
//}

//type LinuxSpec struct {
//	NamespaceSpec []NamespacesSpec `json:"namespaces"`
//}

//type RuntimeSpec struct {
//	Linux LinuxSpec `json:"linux"`
//}

//type RuntimeOptions struct {
//	SystemdCGroup bool `json:"systemd_cgroup"`
//}

type Linux struct {
	CgroupParent string `json:"cgroup_parent"`
}

type Config struct {
	Linux Linux `json:"linux"`
}

type sandboxInfoSpec struct {
	Pid int `json:"pid"`
	//RuntimeSpec    RuntimeSpec    `json:"runtimeSpec"`
	//RuntimeOptions RuntimeOptions `json:"runtimeOptions"`
	Config Config `json:"config"`
}
