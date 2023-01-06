package nettop

import (
	"context"
	"fmt"
	"time"
	"unsafe"

	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func fromV1alpha2VersionResponse(from *v1alpha2.VersionResponse) *runtimeapi.VersionResponse {
	return (*runtimeapi.VersionResponse)(unsafe.Pointer(from))
}

func fromV1alpha2PodSandboxStatusResponse(from *v1alpha2.PodSandboxStatusResponse) *runtimeapi.PodSandboxStatusResponse {
	return (*runtimeapi.PodSandboxStatusResponse)(unsafe.Pointer(from))
}

func fromV1alpha2ContainerStatusResponse(from *v1alpha2.ContainerStatusResponse) *runtimeapi.ContainerStatusResponse {
	return (*runtimeapi.ContainerStatusResponse)(unsafe.Pointer(from))
}

func v1alpha2ContainerStatsFilter(from *runtimeapi.ContainerStatsFilter) *v1alpha2.ContainerStatsFilter {
	return (*v1alpha2.ContainerStatsFilter)(unsafe.Pointer(from))
}

func fromV1alpha2ListContainerStatsResponse(from *v1alpha2.ListContainerStatsResponse) *runtimeapi.ListContainerStatsResponse {
	return (*runtimeapi.ListContainerStatsResponse)(unsafe.Pointer(from))
}

func fromV1alpha2ListPodSandboxResponse(from *v1alpha2.ListPodSandboxResponse) *runtimeapi.ListPodSandboxResponse {
	return (*runtimeapi.ListPodSandboxResponse)(unsafe.Pointer(from))
}

func fromV1alpha2ListContainersResponse(from *v1alpha2.ListContainersResponse) *runtimeapi.ListContainersResponse {
	return (*runtimeapi.ListContainersResponse)(unsafe.Pointer(from))
}

func fromV1alpha2ListPodSandboxStatsResponse(from *v1alpha2.ListPodSandboxStatsResponse) *runtimeapi.ListPodSandboxStatsResponse {
	return (*runtimeapi.ListPodSandboxStatsResponse)(unsafe.Pointer(from))
}

func fromV1alpha2PodSandboxStats(from *v1alpha2.PodSandboxStats) *runtimeapi.PodSandboxStats {
	return (*runtimeapi.PodSandboxStats)(unsafe.Pointer(from))
}

func fromV1alpha2StatusResponse(from *v1alpha2.StatusResponse) *runtimeapi.StatusResponse {
	return (*runtimeapi.StatusResponse)(unsafe.Pointer(from))
}

func v1alpha2ContainerFilter(from *runtimeapi.ContainerFilter) *v1alpha2.ContainerFilter {
	return (*v1alpha2.ContainerFilter)(unsafe.Pointer(from))
}

func v1alpha2PodSandboxFilter(from *runtimeapi.PodSandboxFilter) *v1alpha2.PodSandboxFilter {
	return (*v1alpha2.PodSandboxFilter)(unsafe.Pointer(from))
}

func v1alpha2PodSandboxStatsFilter(from *runtimeapi.PodSandboxStatsFilter) *v1alpha2.PodSandboxStatsFilter {
	return (*v1alpha2.PodSandboxStatsFilter)(unsafe.Pointer(from))
}

// verifySandboxStatus verified whether all required fields are set in PodSandboxStatus.
func verifySandboxStatus(status *runtimeapi.PodSandboxStatus) error {
	if status.Id == "" {
		return fmt.Errorf("status.Id is not set")
	}

	if status.Metadata == nil {
		return fmt.Errorf("status.Metadata is not set")
	}

	metadata := status.Metadata
	if metadata.Name == "" || metadata.Namespace == "" || metadata.Uid == "" {
		return fmt.Errorf("metadata.Name, metadata.Namespace or metadata.Uid is not in metadata %q", metadata)
	}

	if status.CreatedAt == 0 {
		return fmt.Errorf("status.CreatedAt is not set")
	}

	return nil
}

// getContextWithTimeout returns a context with timeout.
func getContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// getContextWithCancel returns a context with cancel.
func getContextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// verifyContainerStatus verified whether all required fields are set in ContainerStatus.
func verifyContainerStatus(status *runtimeapi.ContainerStatus) error {
	if status.Id == "" {
		return fmt.Errorf("status.Id is not set")
	}

	if status.Metadata == nil {
		return fmt.Errorf("status.Metadata is not set")
	}

	metadata := status.Metadata
	if metadata.Name == "" {
		return fmt.Errorf("metadata.Name is not in metadata %q", metadata)
	}

	if status.CreatedAt == 0 {
		return fmt.Errorf("status.CreatedAt is not set")
	}

	if status.Image == nil || status.Image.Image == "" {
		return fmt.Errorf("status.Image is not set")
	}

	if status.ImageRef == "" {
		return fmt.Errorf("status.ImageRef is not set")
	}

	return nil
}
