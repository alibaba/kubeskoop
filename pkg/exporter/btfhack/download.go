package btfhack

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
)

const (
	EnvBTFDownloadURL = "BTF_DOWNLOAD_URL"
	OpenBTFURL        = "https://mirrors.openanolis.cn/coolbpf/btf/"
)

func downloadBTFOnline(btfDstPath string) (string, error) {
	release, err := bpfutil.KernelRelease()
	if err != nil {
		return "", err
	}
	arch, err := bpfutil.KernelArch()
	if err != nil {
		return "", err
	}

	filename := fmt.Sprintf("vmlinux-%s", release)
	dst := path.Join(btfDstPath, filename)
	urlPath := fmt.Sprintf("%s/%s", arch, filename)
	if envURL, ok := os.LookupEnv(EnvBTFDownloadURL); ok {
		downloadURL, err := url.JoinPath(envURL, urlPath)
		if err != nil {
			return "", err
		}
		err = downloadTo(downloadURL, dst)
		if err == nil {
			log.Printf("Downloaded btf file from %s", downloadURL)
			return dst, nil
		}
		log.Printf("Download btf file failed from %s: %s", downloadURL, err)
	}

	downloadURL, err := url.JoinPath(OpenBTFURL, urlPath)
	if err != nil {
		return "", err
	}
	err = downloadTo(downloadURL, dst)
	if err != nil {
		return "", fmt.Errorf("download btf file failed from %s: %w", downloadURL, err)
	}
	return dst, nil
}

func downloadTo(url, dst string) error {
	tr := &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   1 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
	}

	client := http.Client{
		Timeout:   50 * time.Second,
		Transport: tr,
	}

	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("got status code %d", res.StatusCode)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, res.Body)
	if err != nil {
		return err
	}
	return nil
}
