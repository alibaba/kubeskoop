# 介绍

## 快速开始

Skoop exporter 依赖 Kubernetes version >= 1.18。

Skoop exporter 容器会有以下行为需要额外的配置：

* 需要运行在特权模式下，用于下发eBPF采集程序。
* 需要访问CRI接口与节点的/proc路径，用于获取节点的信息。
* 需要访问bpffs(通常挂载到/sys/fs)和debugfs(通常挂载到/sys/kernel/debug/),用于获取Linux内核的信息。

## 安装

### 使用 Skoop Bundle 安装

Skoop exporter提供了一个可以快速部署的配置，包含以下组件：

* Skoop exporter组件。
* 单副本的Prometheus组件与Grafana组件，Grafana Loki组件。
* Prometheus和Grafana的NodePort服务。

通过以下步骤，可以在Kubernetes集群中快速部署Skoop exporter及其与Prometheus，Grafana和Loki构成的可观测性组合：

```shell
kubectl apply -f https://github.com/alibaba/kubeskoop/deploy/skoopbundle.yaml
```

通过以下步骤，确认安装完成以及获取访问入口：

```shell
# 查看Skoop exporter的运行状态
kubectl get pod -n skoop -l app=skoop-exporter -o wide

# 查看Probe采集探针的运行状态
curl 10.1.17.239:9102/status |jq .

# 获取Prometheus服务的入口
kubectl get service -n skoop prometheus-service -o wide

# 获取Grafana控制台的访问入口
kubectl get service -n skoop grafana -o wide
```

***备注: skoopbundle.yaml以最小副本方式启动，不适用于生产环境***

### 使用 Helm 安装

Skoop exporter可以通过Helm进行部署：

```shell
# 添加skoop charts repo
helm repo add kubeskoop https://github.com/alibaba/kubeskoop/charts

# 首次执行时，需要更新helm repo缓存
helm repo update

# 安装skoop exporter
helm install skoop-exporter kubeskoop/skoop-exporter
```

如果需要调试Helm Charts信息，可以通过本地安装：

```shell
# 获取skoop exporter代码仓库
git clone https://github.com/alibaba/kubeskoop.git

# 进行本地安装
helm install --set namespace=kube-system skoop-exporter ./kubeskoop/deploy/skoop-exporter-0.1.0.tgz --debug
```

Skoop-exporter以DeamonSet方式部署在集群中，可以通过以下方式验证是否正常工作：

```shell
# 查看Skoop exporter的运行状态
kubectl get pod -n skoop -l app=skoop-exporter -o wide

# 获取到pod的信息后，可以通过apiserver查看Probe采集探针的运行状态
kubectl get --raw /api/v1/namespaces/{{skoop-exporter的pod namespace}}/pods/{{skoop-exporter的pod name}}:9102/proxy/status |jq .

# 如果可以直接访问skoop-exporter实例，也可以直接查看Probe的运行状态
curl {{skoop-exporter的pod ip}}:9102/status |jq .
```

### 配置 Helm 参数

| Setting                            | Description                                                                                                          | Default                            |
|------------------------------------|----------------------------------------------------------------------------------------------------------------------|------------------------------------|
| name                               | Skoop-exporter daemonset name                                                                                        | `skoop-exporter`                   |
| namespace                          | The namespace of skoop-exporter workload                                                                             | `skoop`                            |
| debugmode                          | Enable the debugmode of skoop-exporter, with debug interface, debug log level and pprof support                      | `false`                            |
| config.serverPort                  | Metric server port, provide HTTP service                                                                             | 9102                               |
| config.metricsProbes               | Metric probes to enable                                                                                              | refer to the probe guide           |
| config.eventProbes                 | Event probes to enable                                                                                               | refer to the probe guide           |
| config.eventSinks                  | Sink config for events, stderr/file/loki are supported now                                                           | 15                                 |

## 配置

Skoop-exporter的配置是默认由与workload相同命名空间下的ConfigMap对象inspector-config进行管理，通过一下方式可以进行修改：

```shell
# 修改命名空间为实际生效的命名空间
kubectl edit cm -n skoop inspector-config
```

Skoop-exporter支持的配置项如下:

| Setting                            | Description                                                                                                          | Default                            |
|------------------------------------|----------------------------------------------------------------------------------------------------------------------|------------------------------------|
| debugmode                          | Enable the debugmode of skoop-exporter, with debug interface, debug log level and pprof support                      | `false`                            |
| port                               | metrics server port                                                                                                  | 9102                               |
| metrics.probes                     | Metric probes to enable                                                                                              | refer to the probe guide           |
| event.probes                       | Event  probes to enable                                                                                              | refer to the probe guide           |
| event.sink                         | Sink config for events, stderr/file/loki are supported now                                                           | refer to the probe guide           |

可以选择配置的probe如下，他们的详细指标和事件，参见reference_guide：

| Setting                            | Type                                                                                                                 | Default  Enable                    |
|------------------------------------|----------------------------------------------------------------------------------------------------------------------|------------------------------------|
| netdev                             | Infromation of network device, support metrics                                                                       | `true`                             |
| io                                 | Infromation of io syscalls, support metrics                                                                          | `true`                             |
| socketlatency                      | Latency statistics of socket recv/send syscalls, support metrics  and events                                         | `true`                             |
| packetloss                         | Infromation of io syscalls, support metrics                                                                          | `true`                             |
| net_softirq                        | Network softirq sched and excute latency, support metrics and events                                                 | `true`                             |
| tcpext                             | Infromation of tcp netstat, support metrics                                                                          | `true`                             |
| tcpsummary                         | Infromation of tcp detail connection statistic, support metrics                                                      | `true`                             |
| tcp                                | Infromation of snmp statistics, support metrics                                                                      | `true`                             |
| sock                               | Statistics of sock allocation and memory usage, support metrics                                                      | `true`                             |
| softnet                            | Statistics of softnet packet processing, support metrics                                                             | `true`                             |
| udp                                | Infromation of udp datagram processing, support metrics  and events                                                  | `true`                             |
| virtcmdlatency                     | Infromation of virtio-net command excution, support metrics  and events                                              | `true`                             |
| kernellatency                      | Infromation of linux kernel sk_buff handle latency, support metrics and events                                       | `false`                            |
| tcpreset                           | Infromation of tcp stream aborting with reset flag, support events                                                   | `true`                             |
| conntrack                          | Infromation of conntrack information, support metrics                                                                | `fasle`                            |
| biolatency                         | Infromation of block device io latency, support events                                                               | `false`                            |
| netif_txlatency                    | Infromation of network interface queuing and sending latency, support metrics and events                             | `false`                            |
