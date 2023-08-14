# Vagrantfile for KubeSkoop exporter

This `Vagrantfile` can setup a 3 node kubernetes cluster (1 master, 2 worker) with `flannel` plugin and KubeSkoop exporter for your tests.

Before you start, you need to install [Vagrant](https://developer.hashicorp.com/vagrant/docs/installation) and [VirtualBox](https://www.virtualbox.org/wiki/Downloads) first.

## Run KubeSkoop exporter with Vagrant

When you installed Vagrant and VirtualBox, you can clone the `kubeskoop repo`, move to this folder, and run `vagrant up`.

```shell
git clone git@github.com:alibaba/kubeskoop.git
cd kubeskoop/deploy/vagrant-exporter
vagrant up
```

It may take a white to set up 3 virtual machines to build a kubernetes cluster.

## Manage cluster with `kubectl`

When your cluster is ready, you can ssh into the master node to take a look at the cluster.

```shell
vagrant ssh master
# on master node
kubectl get pod -n kube-system
```

KubeSkoop are installed in `kubeskoop` namespace.

```shell
# on master node
kubectl get pod -n kubeskoop
```

## Access the Grafana on host machine

When all pods under `kubeskoop` namespace are ready, you can now access the Grafana via [http://127.0.0.1:8080](http://127.0.0.1:8080) on your host machine.

The default user is `admin`, and password is `kubeskoop`.
