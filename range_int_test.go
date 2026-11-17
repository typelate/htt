package htt_test

import "testing"

func TestRangeIntegerKeepsItsType(t *testing.T) {
	const tmpl = `{{range $v := .}}{{printf "%T" $v}} {{end}}`
	tests := []struct {
		name string
		val  any
		want string
	}{
		{name: "int8", val: int8(3), want: "int8 int8 int8 "},
		{name: "uint16", val: uint16(3), want: "uint16 uint16 uint16 "},
		{name: "int64", val: int64(3), want: "int64 int64 int64 "},
		{name: "uintptr", val: uintptr(3), want: "uintptr uintptr uintptr "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := execText(t, "range", tmpl, test.val)
			if got != test.want {
				t.Errorf("range over %T(%v) = %q, want %q", test.val, test.val, got, test.want)
			}
		})
	}
}
