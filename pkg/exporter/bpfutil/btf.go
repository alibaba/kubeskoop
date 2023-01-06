package bpfutil

import (
	"debug/elf"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cilium/ebpf/btf"
	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

const (
	BTFPATH      = "/etc/net-exporter/"
	bpfSharePath = "/etc/net-exporter/btf/"
)

var (
	inspMapPath = "/sys/fs/bpf/inspector/"
)

func GetBtfFile() (string, error) {
	v, err := KernelRelease()
	if err != nil {
		return "", err
	}
	// prefer to use self-hosted btf file
	selfpath := fmt.Sprintf("%svmlinux-%s", BTFPATH, v)
	if _, err := os.Stat(selfpath); os.IsNotExist(err) {
		sharepath := fmt.Sprintf("%svmlinux-%s", bpfSharePath, v)
		if _, err := os.Stat(sharepath); os.IsNotExist(err) {
			return "", err
		}
		return sharepath, nil
	}
	slog.Default().Info("found btf file", "path", selfpath)
	return selfpath, nil
}

func KernelRelease() (string, error) {
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return "", fmt.Errorf("uname failed: %w", err)
	}

	return unix.ByteSliceToString(uname.Release[:]), nil
}

// LoadBTFSpecOrNil once error occurs in load process, return nil and use system raw spec instead
func LoadBTFSpecOrNil() *btf.Spec {
	btffile, err := findBTFFileWithPath(BTFPATH)
	if btffile == "" || err != nil {
		slog.Default().Info("load btf file", "path", BTFPATH, "err", err)
		btffile, err = findBTFFileWithPath(bpfSharePath)
		if btffile == "" || err != nil {
			slog.Default().Info("load btf file", "path", bpfSharePath, "err", err)
		}
	}

	if btffile == "" {
		return nil
	}

	spec, err := loadBTFSpec(btffile)
	if err != nil {
		slog.Default().Info("btf load spec failed", "file", btffile, "err", err)
		return nil
	}
	slog.Default().Info("btf file loaded", "file", btffile)
	return spec
}

func FindBTF(path string) (string, error) {
	return findBTFFileWithPath(path)
}

func findBTFFileWithPath(path string) (string, error) {
	path = filepath.Clean(path)

	v, err := KernelRelease()
	if err != nil {
		return "", err
	}

	btffile := fmt.Sprintf("%s/vmlinux-%s", path, v)
	if _, err := os.Stat(btffile); os.IsNotExist(err) {
		return "", fmt.Errorf("btf %s not found", btffile)
	}

	return btffile, err
}

func LoadBTFFromFile(file string) (*btf.Spec, error) {
	return loadBTFSpec(file)
}

func loadBTFSpec(file string) (*btf.Spec, error) {
	fh, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fh.Close()

	spec, err := btf.LoadSpecFromReader(fh)
	if err == nil {
		return spec, nil
	}

	// if load spec with err, try to extract btf section directly
	btfelf, err := getELFFileFromReader(fh)
	if err != nil {
		return nil, fmt.Errorf("read bare elf:%s", err)
	}

	var (
		btfSection *elf.Section
	)

	for _, sec := range btfelf.Sections {
		switch sec.Name {
		case ".BTF":
			btfSection = sec
		default:
		}
	}

	if btfSection == nil {
		return nil, fmt.Errorf("read bare elf: no .BTF section in %s", file)
	}

	if btfSection.ReaderAt == nil {
		return nil, fmt.Errorf("compressed BTF is not supported")
	}

	return btf.LoadSpecFromReader(btfSection.ReaderAt)
}

func getELFFileFromReader(r io.ReaderAt) (safe *elf.File, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		safe = nil
		err = fmt.Errorf("reading ELF file panicked: %s", r)
	}()

	file, err := elf.NewFile(r)
	if err != nil {
		return nil, err
	}

	return file, nil
}
