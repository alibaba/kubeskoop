package utils

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNormalize(t *testing.T) {
	pod := v1.Pod{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "name",
			Namespace: "ns",
		},
	}
	normalPod := Normalize("pod", &pod)
	if normalPod != "pod/ns/name" {
		t.Errorf("unexpect normalize result: %v", normalPod)
		t.Fail()
	}

	svc := v1.Service{
		TypeMeta: meta_v1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "name",
			Namespace: "ns",
		},
	}
	normalSvc := Normalize("service", &svc)
	if normalSvc != "service/ns/name" {
		t.Errorf("unexpect normalize result: %v", normalSvc)
		t.Fail()
	}
}
