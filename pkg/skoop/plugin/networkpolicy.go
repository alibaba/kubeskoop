package plugin

import (
	"context"
	"fmt"
	"net"

	"github.com/alibaba/kubeskoop/pkg/skoop/k8s"
	model "github.com/alibaba/kubeskoop/pkg/skoop/model"
	"github.com/alibaba/kubeskoop/pkg/skoop/service"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
)

type NetworkPolicyHandler interface {
	CheckNetworkPolicy(src, dst model.Endpoint, protocol model.Protocol) ([]model.Suspicion, error)
}

type networkPolicy struct {
	// serviceAddrSkipCidrRule
	// skip check service address in egress
	// e.g. Calico
	serviceAddrSkipCidrRule bool
	// inClusterAddrEmitCidrRule
	// if true: then in cluster resource, if label not match, reject it even ip in cidr block
	// e.g. Cilium
	inClusterAddrEmitCidrRule bool

	policies []v1.NetworkPolicy
	ipCache  *k8s.IPCache
	k8sCli   *clientset.Clientset
	service  service.Processor
}

func NewNetworkPolicy(serviceAddrSkipCidrRule bool, inClusterAddrEmitCidrRule bool, ipCache *k8s.IPCache, k8sCli *clientset.Clientset, service service.Processor) (NetworkPolicyHandler, error) {
	np := &networkPolicy{serviceAddrSkipCidrRule: serviceAddrSkipCidrRule, inClusterAddrEmitCidrRule: inClusterAddrEmitCidrRule, ipCache: ipCache, k8sCli: k8sCli, service: service}
	policyList, err := np.k8sCli.NetworkingV1().NetworkPolicies("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	np.policies = policyList.Items
	return np, nil
}

func (np *networkPolicy) CheckNetworkPolicy(src, dst model.Endpoint, protocol model.Protocol) ([]model.Suspicion, error) {
	var denies []*v1.NetworkPolicy
	var ret []model.Suspicion
	if src.Type == model.EndpointTypePod {
		pod, err := np.ipCache.GetPodFromIP(src.IP)
		if err != nil {
			return nil, fmt.Errorf("error get pod from ip ipCache: %v", err)
		}
		if dst.Type != model.EndpointTypeService && dst.Type != model.EndpointTypeLoadbalancer {
			nps, err := np.checkEgress(pod, dst, protocol)
			if err != nil {
				return nil, fmt.Errorf("error check egress policies: %v", err)
			}
			denies = append(denies, nps...)
		} else if !np.serviceAddrSkipCidrRule {
			// pod -> svc
			nps, err := np.checkEgress(pod, dst, protocol)
			if err != nil {
				return nil, fmt.Errorf("error check egress policies: %v", err)
			}
			denies = append(denies, nps...)
		}
	}
	if dst.Type == model.EndpointTypePod {
		pod, err := np.ipCache.GetPodFromIP(dst.IP)
		if err != nil {
			return nil, fmt.Errorf("error get pod from ip ipCache: %v", err)
		}
		nps, err := np.checkIngress(pod, src, protocol)
		if err != nil {
			return nil, fmt.Errorf("error check ingress policies: %v", err)
		}
		denies = append(denies, nps...)
	}
	if dst.Type == model.EndpointTypeService || dst.Type == model.EndpointTypeLoadbalancer {
		svc, err := np.ipCache.GetServiceFromIP(dst.IP)
		if err != nil {
			return nil, fmt.Errorf("error get service(%v) from ip ipCache: %v", dst.IP, err)
		}
		backends := np.service.Process(model.Packet{
			Src:      net.ParseIP(src.IP),
			Sport:    src.Port,
			Dst:      net.ParseIP(dst.IP),
			Dport:    dst.Port,
			Protocol: protocol,
		}, svc, nil)
		for _, backend := range backends {
			if backend.IP == dst.IP {
				return nil, fmt.Errorf("service network loop")
			}
			backendType, err := np.ipCache.GetIPType(backend.IP)
			if err != nil {
				return nil, err
			}
			dst := model.Endpoint{
				IP:   backend.IP,
				Type: backendType,
				Port: backend.Port,
			}
			sub, err := np.CheckNetworkPolicy(src, dst, protocol)
			if err != nil {
				return ret, err
			}
			ret = append(ret, sub...)
		}
	}

	for _, np := range denies {
		ret = append(ret, model.Suspicion{Level: model.SuspicionLevelCritical, Message: fmt.Sprintf("network policy %v/%v deny the packet from %v to(%v) %v:%v",
			np.Namespace, np.Name,
			src.IP, protocol, dst.IP, dst.Port),
		})
	}
	return ret, nil
}

func (np *networkPolicy) checkEgress(pod *corev1.Pod, dst model.Endpoint, protocol model.Protocol) ([]*v1.NetworkPolicy, error) {
	var denies []*v1.NetworkPolicy
	for _, policy := range np.policies {
		match, err := np.policyMatchPod(&policy, pod)
		if err != nil {
			return nil, err
		}
		if match {
			dstPodOrNil, err := np.ipCache.GetPodFromIP(dst.IP)
			if err != nil {
				return nil, err
			}
			deny, err := np.checkEgressPolicyVerdict(&policy, dstPodOrNil, dst, protocol)
			if err != nil {
				return nil, err
			} else if deny {
				denies = append(denies, &policy)
			}
		}
	}
	return denies, nil
}

func toV1Protocol(p model.Protocol) corev1.Protocol {
	switch p {
	case model.TCP:
		return corev1.ProtocolTCP
	case model.UDP:
		return corev1.ProtocolUDP
	default:
		return corev1.ProtocolTCP
	}
}

func (np *networkPolicy) containsPortWithProtocol(port uint16, protocol model.Protocol, ports []v1.NetworkPolicyPort) bool {
	// no port means no limit
	if len(ports) == 0 {
		return true
	}

	// todo: support named port
	for _, p := range ports {
		if p.Port == nil {
			continue
		}

		policyProtocol := corev1.ProtocolTCP
		// If not specified, this field defaults to TCP.
		if p.Protocol != nil {
			policyProtocol = *p.Protocol
		}

		if policyProtocol != toV1Protocol(protocol) {
			continue
		}

		from := p.Port.IntVal
		if from <= 0 {
			continue
		}

		to := from
		if p.EndPort != nil {
			to = *p.EndPort
		}

		if port >= uint16(from) && port <= uint16(to) {
			return true
		}
	}

	return false
}

func (np *networkPolicy) checkEgressPolicyVerdict(policy *v1.NetworkPolicy, dstPod *corev1.Pod, dst model.Endpoint, protocol model.Protocol) (deny bool, err error) {
	if !np.hasPolicyType(policy, v1.PolicyTypeEgress) {
		return false, nil
	}
	for _, egressRule := range policy.Spec.Egress {
		if !np.containsPortWithProtocol(dst.Port, protocol, egressRule.Ports) {
			continue
		}

		for _, to := range egressRule.To {
			if dstPod != nil {
				if to.PodSelector != nil {
					if dstPod.GetNamespace() != policy.GetNamespace() {
						continue
					}
					selector, err := metav1.LabelSelectorAsSelector(to.PodSelector)
					if err != nil {
						return false, err
					}
					if selector.Empty() || selector.Matches(labels.Set(dstPod.Labels)) {
						return false, nil
					}
				} else if to.NamespaceSelector != nil {
					selector, err := metav1.LabelSelectorAsSelector(to.NamespaceSelector)
					if err != nil {
						return false, err
					}
					if selector.Empty() {
						return false, nil
					}
					dstNamespace, err := np.k8sCli.CoreV1().Namespaces().Get(context.Background(), dstPod.GetNamespace(), metav1.GetOptions{})
					if err != nil {
						return false, err
					}
					if selector.Matches(labels.Set(dstNamespace.Labels)) {
						return false, nil
					}
				}
			}
			if to.IPBlock != nil {
				if dstPod != nil && np.inClusterAddrEmitCidrRule {
					continue
				}
				contains, err := np.strCidrContainsIP(to.IPBlock.CIDR, dst.IP)
				if err != nil {
					return false, err
				}
				if contains {
					var ipExcept bool
					for _, exceptCIDR := range to.IPBlock.Except {
						except, err := np.strCidrContainsIP(exceptCIDR, dst.IP)
						if err != nil {
							return false, err
						}
						if except {
							ipExcept = true
							break
						}
					}
					if !ipExcept {
						return false, nil
					}
				}
			}
		}
	}

	return true, nil
}

func (np *networkPolicy) strCidrContainsIP(cidr, ip string) (bool, error) {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, err
	}
	return subnet.Contains(net.ParseIP(ip)), nil
}

func (np *networkPolicy) hasPolicyType(policy *v1.NetworkPolicy, pt v1.PolicyType) bool {
	for _, p := range policy.Spec.PolicyTypes {
		if p == pt {
			return true
		}
	}
	return false
}

func (np *networkPolicy) policyMatchPod(policy *v1.NetworkPolicy, pod *corev1.Pod) (bool, error) {
	if policy.GetNamespace() != pod.GetNamespace() {
		return false, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.PodSelector)
	if err != nil {
		return false, err
	}
	if !selector.Empty() && !selector.Matches(labels.Set(pod.Labels)) {
		return false, nil
	}
	return true, nil
}

func (np *networkPolicy) checkIngress(pod *corev1.Pod, src model.Endpoint, protocol model.Protocol) ([]*v1.NetworkPolicy, error) {
	var denies []*v1.NetworkPolicy
	for _, policy := range np.policies {
		match, err := np.policyMatchPod(&policy, pod)
		if err != nil {
			return nil, err
		}

		if match {
			srcPodOrNil, err := np.ipCache.GetPodFromIP(src.IP)
			if err != nil {
				return nil, fmt.Errorf("error get src pod(%v) from ip cache: %v", src.IP, err)
			}
			deny, err := np.checkIngressPolicyVerdict(&policy, srcPodOrNil, src, protocol)
			if err != nil {
				return nil, err
			}

			if deny {
				denies = append(denies, &policy)
			}
		}
	}
	return denies, nil
}

func (np *networkPolicy) checkIngressPolicyVerdict(policy *v1.NetworkPolicy, srcPod *corev1.Pod, src model.Endpoint, protocol model.Protocol) (deny bool, err error) {
	if !np.hasPolicyType(policy, v1.PolicyTypeIngress) {
		return false, nil
	}
	for _, ingressRule := range policy.Spec.Ingress {
		if !np.containsPortWithProtocol(src.Port, protocol, ingressRule.Ports) {
			continue
		}
		for _, from := range ingressRule.From {
			if srcPod != nil {
				if from.PodSelector != nil {
					if srcPod.GetNamespace() != policy.GetNamespace() {
						continue
					}

					selector, err := metav1.LabelSelectorAsSelector(from.PodSelector)
					if err != nil {
						return false, err
					}

					if selector.Empty() || selector.Matches(labels.Set(srcPod.Labels)) {
						return false, nil
					}

				} else if from.NamespaceSelector != nil {
					selector, err := metav1.LabelSelectorAsSelector(from.NamespaceSelector)
					if err != nil {
						return false, err
					}

					if selector.Empty() {
						return false, nil
					}
					srcNamespace, err := np.k8sCli.CoreV1().Namespaces().Get(context.Background(), srcPod.GetName(), metav1.GetOptions{})
					if err != nil {
						return false, err
					}

					if selector.Matches(labels.Set(srcNamespace.Labels)) {
						return false, nil
					}
				}
			}
			if from.IPBlock != nil {
				if srcPod != nil && np.inClusterAddrEmitCidrRule {
					continue
				}
				contains, err := np.strCidrContainsIP(from.IPBlock.CIDR, src.IP)
				if err != nil {
					return false, err
				}

				if !contains {
					continue
				}
				var ipExcept bool
				for _, exceptCIDR := range from.IPBlock.Except {
					except, err := np.strCidrContainsIP(exceptCIDR, src.IP)
					if err != nil {
						return false, err
					}

					if except {
						ipExcept = true
						break
					}
				}
				if !ipExcept {
					return false, nil
				}
			}
		}
	}

	return true, nil
}
