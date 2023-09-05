package bpfutil

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

type KernelSymbol struct {
	start      uint64
	symboltype string
	symbol     string
	module     string
	offset     int
}

func (k *KernelSymbol) GetAddr() uint64 {
	return k.start
}

func (k *KernelSymbol) GetName() string {
	return k.symbol
}

func (k *KernelSymbol) GetExpr() string {
	return fmt.Sprintf("%s+0x%X", k.symbol, k.offset)
}

var kallsyms []KernelSymbol

func init() {
	kallsyms = []KernelSymbol{}
	if err := getAllSyms(); err != nil {
		return
	}
}

func GetSymByPt(addr string) (*KernelSymbol, error) {
	var pt uint64
	if strings.HasPrefix(addr, "0x") {
		addr = addr[2:]
		res, err := strconv.ParseUint(addr, 16, 64)
		if err != nil {
			return nil, err
		}
		pt = res
	} else {
		res, err := strconv.ParseUint(addr, 10, 64)
		if err != nil {
			return nil, err
		}
		pt = res
	}

	if pt > kallsyms[len(kallsyms)-1].start {
		return nil, errors.New("addr out of range")
	}

	for idx := range kallsyms {
		if kallsyms[idx].start <= pt && kallsyms[idx+1].start > pt {
			ks := KernelSymbol{
				start:      kallsyms[idx].start,
				symboltype: kallsyms[idx].symbol,
				symbol:     kallsyms[idx].symbol,
				offset:     int(pt - kallsyms[idx].start),
				module:     kallsyms[idx].module,
			}
			return &ks, nil
		}
	}

	return nil, errors.New("addr not found")
}

var locationCache = expirable.NewLRU[uint64, KernelSymbol](100, nil, 0)

// GetSymPtFromBpfLocation return symbol struct/offset/error with bpf location
func GetSymPtFromBpfLocation(pt uint64) (*KernelSymbol, error) {
	sym, ok := locationCache.Get(pt)
	if ok {
		return &sym, nil
	}
	// since getAllSyms is already sorted, we can do a binary search here
	var i, j, mid int
	var midVal uint64
	j = len(kallsyms) - 1
	found := -1
	for i < j {
		mid = i + (j-i)/2
		midVal = kallsyms[mid].start
		if pt < midVal {
			j = mid
		} else if pt > midVal {
			i = mid + 1
		} else {
			found = mid
			break
		}
	}
	if found == -1 {
		if i >= 1 && pt > kallsyms[i-1].start && pt < kallsyms[i].start {
			found = i - 1
		}
		if pt >= kallsyms[i].start {
			found = i
		}
	}
	if found == -1 {
		return nil, errors.New("addr not found")
	}
	ks := KernelSymbol{
		start:      kallsyms[found].start,
		symboltype: kallsyms[found].symbol,
		symbol:     kallsyms[found].symbol,
		offset:     int(pt - kallsyms[found].start),
		module:     kallsyms[found].module,
	}
	locationCache.Add(pt, ks)
	return &ks, nil
}

func getAllSyms() error {
	f, err := os.Open("/proc/kallsyms")
	if err != nil {
		return err
	}

	r := bufio.NewScanner(f)
	for r.Scan() {
		rawsym := strings.Split(r.Text(), " ")
		pt, err := strconv.ParseUint(rawsym[0], 16, 64)
		if err != nil {
			fmt.Println(err)
			continue
		}
		ks := KernelSymbol{
			start:      pt,
			symboltype: rawsym[1],
			symbol:     rawsym[2],
		}
		if len(rawsym) == 4 {
			ks.module = rawsym[3]
		} else {
			ks.module = "kernel"
		}

		kallsyms = append(kallsyms, ks)
	}

	sort.Slice(kallsyms, func(i, j int) bool {
		return kallsyms[i].GetAddr() < kallsyms[j].GetAddr()
	})

	return nil
}

func GetSymsStrByStack(stack uint32, stackmap *ebpf.Map) ([]string, error) {
	addrs := [10]uint64{}

	if err := stackmap.Lookup(stack, &addrs); err != nil {
		return nil, err
	}

	callers := []string{}
	for _, addr := range addrs {
		sym, err := GetSymPtFromBpfLocation(addr)
		if err != nil {
			continue
		}

		callerStr := fmt.Sprintf("%s+0x%X", sym.GetName(), sym.offset)
		callers = append(callers, callerStr)
	}

	return callers, nil
}

func GetSymsByStack(stack uint32, stackmap *ebpf.Map) ([]KernelSymbol, error) {
	addrs := [10]uint64{}

	if err := stackmap.Lookup(stack, &addrs); err != nil {
		return nil, err
	}

	callers := []KernelSymbol{}
	for _, addr := range addrs {
		sym, err := GetSymPtFromBpfLocation(addr)
		if err != nil {
			continue
		}

		if sym == nil {
			continue
		}

		callers = append(callers, *sym)
	}

	return callers, nil
}
