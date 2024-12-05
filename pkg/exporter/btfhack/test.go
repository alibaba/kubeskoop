package btfhack

import (
	"log"

	"github.com/alibaba/kubeskoop/pkg/exporter/bpfutil"
	"github.com/alibaba/kubeskoop/pkg/exporter/testbtf"

	"github.com/cilium/ebpf/btf"
	"github.com/spf13/cobra"
)

// testCmd represents the test command
var (
	testCmd = &cobra.Command{
		Use:   "test",
		Short: "test btf support locally",
		Run: func(_ *cobra.Command, _ []string) {
			if btfSrcPath == "" {
				btfSrcPath = defaultBTFPath
			}

			file, err := bpfutil.FindBTFFileWithPath(btfSrcPath)
			if err != nil {
				log.Printf("failed with %s", err)
				return
			}

			spec, err := bpfutil.LoadBTFFromFile(file)
			if err != nil {
				log.Printf("load btf spec faiild with %s", err)
				return
			}

			if err := testBTFAvailable(spec); err != nil {
				log.Printf("btf test failed: %v", err)
			} else {
				log.Printf("btf test ok")
			}
		},
	}

	btfSrcPath string
)

func init() {
	rootCmd.AddCommand(testCmd)
	flags := testCmd.PersistentFlags()

	flags.StringVarP(&btfSrcPath, "src", "s", "", "btf source file")
}

func testBTFAvailable(spec *btf.Spec) error {
	return testbtf.RunBTFTest(spec)
}
