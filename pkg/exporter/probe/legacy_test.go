package probe

import (
	"reflect"
	"testing"
)

func TestBuildAdditionalLabelsValues1(t *testing.T) {
	type args struct {
		podLabels        map[string]string
		additionalLabels []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{"common", args{map[string]string{"l1": "v1", "l2": "v2"}, []string{"n1=alv1", "n2=alv2"}}, []string{"alv1", "alv2"}},
		{"match", args{map[string]string{"l1": "v1", "l2": "v2"}, []string{"n1=${labels:l1}", "n2=alv2"}}, []string{"v1", "alv2"}},
		{"submatch", args{map[string]string{"l1": "v1", "l2": "v2"}, []string{"n1=prefix_${labels:l1}_${labels:l2}_suffix"}}, []string{"prefix_v1_v2_suffix"}},
		{"nomatch", args{map[string]string{"l1": "v1", "l2": "v2"}, []string{"n1=${labels:ln}"}}, []string{""}},
		{"empty", args{map[string]string{"l1": "v1", "l2": "v2"}, []string{}}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AdditionalLabelValueExpr = []string{}
			err := InitAdditionalLabels(tt.args.additionalLabels)
			if err != nil {
				t.Error(err)
				return
			}

			if got := BuildAdditionalLabelsValues(tt.args.podLabels); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildAdditionalLabelsValues() = %v, want %v", got, tt.want)
			}
		})
	}
}
