package htt_test

import (
	"testing"
)

func TestRangeMapKeyOrder(t *testing.T) {
	tests := []struct {
		name string
		data any
		want string
	}{
		{
			name: "string keys",
			data: map[string]string{
				"delta":   "d",
				"alpha":   "a",
				"echo":    "e",
				"bravo":   "b",
				"charlie": "c",
				"foxtrot": "f",
			},
			want: "alpha:a;bravo:b;charlie:c;delta:d;echo:e;foxtrot:f;",
		},
		{
			name: "int keys",
			data: map[int]string{
				30:  "c",
				-2:  "a",
				7:   "d",
				100: "f",
				0:   "b",
				5:   "e",
			},
			want: "-2:a;0:b;5:e;7:d;30:c;100:f;",
		},
		{
			name: "float keys",
			data: map[float64]string{
				2.5:   "c",
				-1.25: "b",
				10:    "f",
				0.5:   "d",
				3:     "e",
				-7:    "a",
			},
			want: "-7:a;-1.25:b;0.5:d;2.5:c;3:e;10:f;",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := execText(t, test.name, "{{range $k, $v := .}}{{$k}}:{{$v}};{{end}}", test.data)
			if got != test.want {
				t.Errorf("range over %v = %q, want %q", test.data, got, test.want)
			}
		})
	}
}
