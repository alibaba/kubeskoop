package btfhack

import (
	"fmt"
	"log"
	"os/exec"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"

	"github.com/spf13/cobra"
)

const (
	defaultBTFDstPath = "/etc/net-exporter/btf"
)

// cpCmd represents the cp command
var (
	cpCmd = &cobra.Command{
		Use:   "discover",
		Short: "copy or download appropriate btf file to dst path",
		Run: func(_ *cobra.Command, _ []string) {
			if btfSrcPath == "" {
				btfSrcPath = defaultBTFPath
			}
			if btfDstPath == "" {
				btfDstPath = defaultBTFDstPath
			}

			btffile, err := bpfutil.FindBTFFileWithPath(btfSrcPath)
			if err == nil {
				err := copyBtfFile(btffile, btfDstPath)
				if err != nil {
					log.Fatalf("Failed copy btf file: %s\n", err)
				}
				log.Printf("Copy btf file %s to %s succeed\n", btffile, btfDstPath)
				return
			}

			btffile, err = downloadBTFOnline(btfDstPath)
			if err != nil {
				log.Printf("Download btf error: %s\n", err)
				return
			}
			log.Printf("Download btf file %s succeed\n", btffile)
		},
	}

	btfDstPath string
)

func copyBtfFile(path, dstPath string) error {
	cmdToExecute := exec.Command("cp", path, dstPath)
	output, err := cmdToExecute.CombinedOutput()
	if err != nil {
		return fmt.Errorf("load btf with:%s err:%s", output, err)
	}

	log.Printf("load btf %s to %s succeed", path, dstPath)
	return nil
}

func init() {
	rootCmd.AddCommand(cpCmd)

	flags := cpCmd.PersistentFlags()

	flags.StringVarP(&btfSrcPath, "src", "s", "", "btf source file")
	flags.StringVarP(&btfDstPath, "dst", "p", "", "btf destination directory")
}
