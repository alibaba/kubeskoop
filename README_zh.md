# KubeSkoop

![kubeskoop](https://img.shields.io/github/v/tag/alibaba/kubeskoop)
![license](https://img.shields.io/badge/license-Apache-blue)
[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/kubeskoop)](https://goreportcard.com/report/github.com/alibaba/kubeskoop)

[English](./README.md) | 简体中文

- [总览](#总览)
- [快速开始](#快速开始)
- [贡献说明](#贡献说明)
- [联系](#联系方式)
- [License](#license)

## 总览

KubeSkoop是一个Kubernetes网络诊断工具。针对不同的网络插件和IaaS提供商自动构建Kubernetes集群中Pod的网络访问图，结合eBPF对内核关键路径的深度监控和分析，来分析常见的Kubernetes集群网络问题。显著地简化了Kubernetes网络问题的诊断难度。

### 关键特性

#### 一键诊断网络链路

- 诊断kubernetes集群中各种网络访问方式和链路：Pod,Service,Node以及Ingress/Egress Traffic.

- 覆盖完整的Linux协议栈的配置错误场景: Socket,Bridge,Veth,Netfilter,sysctls…

- 支持诊断多种云供应商的IaaS层网络错误配置

#### 深度网络监控

- 通过eBPF实现无侵入的Kernel Montor

- 通过BTF在各种版本的Kernel上直接运行

- 通过标准的Prometheus接口暴露深度监控Metrics

#### 网络异常事件识别

- 数十种网络异常场景的自动分析识别

- 通过Grafana Loki展示网络异常事件

## 快速开始

完整的文档可以直接访问[KubeSkoop.io](https://kubeskoop.io/) 。

### 诊断网络不通问题

#### 诊断命令安装

通过`go install`来安装KubeSkoop的诊断客户端：

```shell
go install github.com/alibaba/kubeskoop/cmd/skoop@latest
```

也可以使用`docker run` 执行`skoop`命令

```shell
docker run -v ~/.kube:/root/.kube --rm kubeskoop/kubeskoop:latest skoop
```

#### 一键诊断

```shell
$ skoop -s 172.18.0.4 -d 10.96.0.10 -p 53 --http # 执行诊断命令，通过src,dst指定源地址和目的地址，使用--http通过本地web服务展示诊断结果
I0118 11:43:23.383446    6280 web.go:97] http server listening on http://127.0.0.1:8080 # 在诊断完成后，将会显示用于查看诊断结果的链接
```

或者通过`docker run`命令执行

```shell
$ docker run -p 8080:8080 -v ~/.kube:/root/.kube kubeskoop/kubeskoop:latest skoop -s 172.18.0.4 -d 10.96.0.10 -p 53 --http # 执行诊断命令，通过src,dst指定源地址和目的地址，使用--http通过本地web服务展示诊断结果
I0118 11:43:23.383446    6280 web.go:97] http server listening on http://127.0.0.1:8080 # 在诊断完成后，将会显示用于查看诊断结果的链接
```

通过浏览器打开`http://127.0.0.1:8080`后可以看到诊断结果：  
![diagnose_web](/docs/images/intro_diagnose_web.jpg)

### 诊断网络抖动和网络性能问题

#### 安装网络监控组件

通过以下步骤，可以在Kubernetes集群中快速部署Skoop exporter及其与Prometheus，Grafana和Loki构成的可观测性组合：

```shell
kubectl apply -f https://raw.githubusercontent.com/alibaba/kubeskoop/main/deploy/skoopbundle.yaml
```

通过以下步骤，确认安装完成以及获取访问入口：

```shell
# 查看KubeSkoop exporter状态
kubectl get pod -n kubeskoop -l app=kubeskoop-exporter -o wide
# 查看探针状态
kubectl get --raw /api/v1/namespaces/kubeskoop/pods/kubeskoop-exporter-t4d9m:9102/proxy/status |jq .
# 获得Prometheus服务的访问入口，服务默认为NodePort类型
kubectl get service -n kubeskoop prometheus-service -o wide
# 获得Grafana控制台服务的访问入口，服务默认为NodePort类型
kubectl get service -n kubeskoop grafana -o wide
```

***备注: skoopbundle.yaml以最小副本方式启动，不适用于生产环境***

#### 查看网络抖动和性能分析

打开Grafana的Service访问入口，打开网络监控的页面，查看对应性能问题时间点的各深度指标的水位情况。例如：
![grafana_performance](/docs/images/monitoring.png)

#### 网络抖动事件

打开Grafana的Service访问入口，打开Loki的页面，查看对应网络抖动时间点对应的事件，以及网络监控页面对应的水位情况。
![grafana_performance](/docs/images/loki_tracing.png)

## 贡献说明

欢迎提交issue和PR来共建此项目。

## 联系方式

- 钉钉群号 (26720020148)

## License

Most source code in KubeSkoop which running on userspace are licensed under the [Apache License, Version 2.0](LICENSE.md).  
The BPF code in `/bpf` directory are licensed under the [GPL v2.0](bpf/COPYING) to compat with Linux kernel helper functions.  
