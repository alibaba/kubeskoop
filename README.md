# KubeSkoop

![kubeskoop](https://img.shields.io/github/v/tag/alibaba/kubeskoop)
![license](https://img.shields.io/badge/license-Apache-blue)
[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/kubeskoop)](https://goreportcard.com/report/github.com/alibaba/kubeskoop)

English | [简体中文](./README_zh.md)

- [Overview](#overview)
- [Quick start](#quick-start)
- [Contributing](#contributing)
- [Contact](#contact)
- [License](#license)

## Overview

KubeSkoop is a kubernetes networking diagnose tool for different CNI plug-ins and IAAS providers.
KubeSkoop automatic construct network traffic graph of Pod in the Kubernetes cluster,
monitoring and analysis of the kernel's critical path by eBPF, to resolve most of Kubernetes cluster network problems.

### Key Features

#### One-Shot Diagnose For Network Broken

- Diagnose in-cluster traffic between Pod,Service,Node and Ingress/Egress Traffic.
- Cover whole linux network stack: Socket,Bridge,Veth,Netfilter,sysctls…
- Support IAAS network probe for cloud providers.

#### In-Depth Kernel Monitor

- eBPF seamless kernel monitor
- CO-RE scripts on series kernel by BTF
- export metrics to standard Prometheus metric API

#### Network Anomaly Event

- support dozens of anomy scenes recognition
- export anomy event to Grafana Loki

## Quick Start

You can view the full documentation from the [KubeSkoop.io](https://kubeskoop.io/).

### One-Shot diagnose persistent network failure

#### Install KubeSkoop command

Through `go install` to install KubeSkoop cli：

```shell
go install github.com/alibaba/kubeskoop/cmd/kubeskoop
```

#### One-Shot Diagnose

```shell
$ kubeskoop -s 172.18.0.4 -d 10.96.0.10 -p 53 --http # Execute the diagnostic command, specify the src,dst, and use --http to provide the diagnostic result through the local web service
I0118 11:43:23.383446    6280 web.go:97] http server listening on http://127.0.0.1:8080 # After the diagnosis is completed, a link to the diagnosis result will be output
```

Open the diagnosis result `http://127.0.0.1:8080` through browser：  
![diagnose_web](/docs/images/intro_diagnose_web.jpg)

### Monitor network jitter and bottlenecks

#### Install monitor components

The KubeSkoop exporter bundles with Prometheus, Grafana, and Loki
can be quickly deployed in a Kubernetes cluster by following these steps:

```shell
kubectl apply -f https://raw.githubusercontent.com/alibaba/kubeskoop/main/deploy/skoopbundle.yaml
```

Confirm that the installation is complete and obtain access through the following steps：

```shell
# View the status of KubeSkoop exporter
kubectl get pod -n kubeskoop -l app=skoop-exporter -o wide
# View the status of Probe collection probes
kubectl get --raw /api/v1/namespaces/kubeskoop/pods/skoop-exporter-t4d9m:9102/proxy/status |jq .
# Obtain the entrance of Prometheus service, which is exposed by NodePort by default
kubectl get service -n kubeskoop prometheus-service -o wide
# Obtain the access entry of the Grafana console, which is exposed by NodePort by default
kubectl get service -n kubeskoop grafana -o wide
```

***Note: skoopbundle.yaml starts with a minimal copy, not suitable for production environments***

#### network performance analysis

Open the NodePort Service of grafana on web browser, open the network monitoring page,
and check the water level of each monitor item corresponding to the time point of the performance problem. For example：  
![grafana_performance](/docs/images/monitoring.png)

#### network jitter & anomy event analysis

Open the NodePort Service of grafana on web browser, open the Loki page,
check the events corresponding to the time point of network jitter and the water level corresponding to the network monitoring page.
![grafana_performance](/docs/images/loki_tracing.png)

## Contributing

Feel free to open issues and pull requests. Any feedback is much appreciated!

## Contact

- DingTalk Group ID(26720020148)

## License

Most source code in KubeSkoop which running on userspace are licensed under the [Apache License, Version 2.0](LICENSE.md).  
The BPF code in `/bpf` directory are licensed under the [GPL v2.0](bpf/COPYING) to compat with Linux kernel helper functions.  
