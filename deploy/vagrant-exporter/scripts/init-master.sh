#!/bin/bash

kubeadm_init() {
sudo kubeadm init --apiserver-advertise-address="$MASTER_IP" --image-repository registry.aliyuncs.com/google_containers --service-cidr=10.96.0.0/12 --pod-network-cidr=10.244.0.0/16
export KUBECONFIG=/etc/kubernetes/admin.conf
}

generate_join_command() {
sudo kubeadm token create --print-join-command | tee /vagrant/join-cluster.sh
chmod +x /vagrant/join-cluster.sh
}

copy_kubeconfig()
{
mkdir /home/vagrant/.kube
sudo cp /etc/kubernetes/admin.conf /vagrant/config
sudo cp /etc/kubernetes/admin.conf /home/vagrant/.kube/config
sudo chown vagrant:vagrant /home/vagrant/.kube/config
}

install_cni() {
kubectl apply -f /vagrant/deploy/kube-flannel.yaml
}

install_kubeskoop() {
kubectl apply -f /vagrant/deploy/skoop-deploy.yaml
}

kubeadm_init
copy_kubeconfig
install_cni
install_kubeskoop
generate_join_command
