package netstack

import "testing"

func TestLookupDefaultIfaceName(t *testing.T) {
	testcases := []struct {
		ifaces   []string
		expected string
	}{
		{
			ifaces:   []string{"eth0", "eno0", "ipvs0", "cali123456"},
			expected: "eth0",
		},
		{
			ifaces:   []string{"enp1s2", "eno0", "ipvs0", "cali123456"},
			expected: "eno0",
		},
		{
			ifaces:   []string{"enp1s1", "ipvs0", "cali123456"},
			expected: "enp1s1",
		},

		{
			ifaces:   []string{"wg0", "ipvs0", "cni0", "aaenp1s1"},
			expected: "",
		},
	}

	for _, c := range testcases {
		var ifs []Interface
		for _, i := range c.ifaces {
			ifs = append(ifs, Interface{Name: i})
		}

		result := LookupDefaultIfaceName(ifs)
		if result != c.expected {
			t.Errorf("excepted testcase %v to be %s, but %s", c.ifaces, c.expected, result)
			t.FailNow()
		}
	}
}
