# Skoop exporter

## INSTALL

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

## VARIABLE

| Setting                            | Description                                                                                                          | Default                            |
|------------------------------------|----------------------------------------------------------------------------------------------------------------------|------------------------------------|
| name                               | Skoop-exporter daemonset name                                                                                        | `skoop-exporter`                   |
| namespace                          | The namespace of skoop-exporter workload                                                                             | `skoop`                            |
| debugmode                          | Enable the debugmode of skoop-exporter, with debug interface, debug log level and pprof support                      | `false`                            |
| config.enableEventServer           | Enable the event server                                                                                              | `false`                            |
| config.enableMetricServer          | Enable the metric server                                                                                             | `true`                             |
| config.remoteLokiAddress           | Set the remote grafana loki endpoint to push events                                                                  | ``                                 |
| config.metricLabelVerbose          | Deliever the detail information of pod in metric label, such as app label, ip                                        | `false`                            |
| config.metricServerPort            | Metric server port, provide HTTP service                                                                             | 9102                               |
| config.eventServerPort             | Event  sever port, provide GRPC service                                                                              | 19102                              |
| config.metricProbes                | Metric probes to enable                                                                                              | refer to the probe guide           |
| config.eventProbes                 | Event probes to enable                                                                                               | refer to the probe guide           |
| config.metricCacheInterval         | Metric cache interval                                                                                                | 15                                 |
