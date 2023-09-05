# KubeSkoop exporter

## INSTALLATION

```shell
# Add KubeSkoop charts repo
helm repo add kubeskoop https://kubeskoop.github.io

# You need to update helm repo info for the first time.
helm repo update

# Install KubeSkoop exporter.
helm install -n kubeskoop --create-namespace kubeskoop-exporter kubeskoop/kubeskoop-exporter
```

You can also install locally if you need to debug the Helm Chart.

```shell
# Clone KubeSkoop to local disk.
git clone https://github.com/alibaba/kubeskoop.git

# Install the helm chart locally.
helm install -n kubeskoop --create-namespace kubeskoop-exporter ./kubeskoop/deploy/kubeskoop-exporter-0.2.0.tgz --debug
```

KubeSkoop exporter are deployed in DaemonSet. You can check the running status via:

```shell
# Get pod running status of KubeSkoop exporter
kubectl get pod -n kubeskoop -l app=kubeskoop-exporter -o wide

# After pods are runing, you can get running status of probes through API server.
kubectl get --raw /api/v1/namespaces/{{kubeskoop-exporter的pod namespace}}/pods/{{kubeskoop-exporter的pod name}}:9102/proxy/status | jq .

# You can also curl it if you have direct access to the pod IP.
curl {{kubeskoop-exporter的pod ip}}:9102/status |jq .
```

## VARIABLES

| Setting                       | Description                                                  | Default                                         |
| ----------------------------- | ------------------------------------------------------------ | ----------------------------------------------- |
| name                          | DaemonSet name of KubeSkoop exporter.                        | `kubeskoop-exporter`                            |
| debugmode                     | Enable `debugmode` for kubeskoop-exporter, with debug interface, debug log level and pprof support. | `false`                                         |
| appName                       | Pod `app` label.                                             | `kubeskoop-exporter`                            |
| runtimeEndpoint               | CRI runtime endpoint socket, you can use  `crictl info | awk -F":" '/containerdEndpoint/ {print $2'` to obtain it. | `/run/containerd/containerd.sock`               |
| image.repository              | Image repository for KubeSkoop exporter container.           | `kubeskoop/kubeskoop`                           |
| image.tag                     | Image tag for KubeSkoop exporter container.                  | `latest`                                        |
| image.imagePullPolicy         | `imagePullPolicy` for KubeSkoop exporter container.          | `Always`                                        |
| initContainer.enabled         | Enable `btfhack` as initContainer to automate discover btf file when kernel does not carry btf information itself. | `true`                                          |
| initContainer.repository      | Image repository for `btfhack` container.                    | `registry.cn-hangzhou.aliyuncs.com/acs/btfhack` |
| initContainer.tag             | Image tag for `btfhack` container.                           | `latest`                                        |
| initContainer.imagePullPolicy | `imagePullPolicy` for `btfhack` container.                   | `Always`                                        |
| config.enableEventServer      | Enable the event server and loki.                            | `false`                                         |
| config.enableMetricServer     | Enable the metric server.                                    | `true`                                          |
| config.remoteLokiAddress      | Set the remote grafana loki endpoint to push events.         | `registry.cn-hangzhou.aliyuncs.com/acs/btfhack` |
| config.metricLabelVerbose     | Deliever the detail information of pod in metric label, such as app label, ip | `false`                                         |
| config.metricServerPort       | Metric server port, provide HTTP service.                    | 9102                                            |
| config.eventServerPort        | Event sever port, provide GRPC service.                      | 19102                                           |
| config.metricProbes           | Metric probes to enable.                                     | Refer to the probe guide.                       |
| config.eventProbes            | Event probes to enable.                                      | Refer to the probe guide.                       |
| config.metricCacheInterval    | Metric cache interval.                                       | 15                                              |
| expose_labels                 | Extra labels to be exposed.                                  | See `values.yaml`.                              |
