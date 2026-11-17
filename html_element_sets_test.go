package htt_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/htt"
)

func TestIsFormattingElement(t *testing.T) {
	for tag, want := range map[string]bool{
		"b":      true,
		"a":      true,
		"strong": true,
		"p":      false,
		"div":    false,
		"":       false,
	} {
		assert.Equal(t, want, htt.IsFormattingElement(tag), "IsFormattingElement(%q)", tag)
	}
}

func TestHasImpliedEndTag(t *testing.T) {
	for tag, want := range map[string]bool{
		"li":   true,
		"p":    true,
		"td":   true,
		"div":  false,
		"span": false,
		"":     false,
	} {
		assert.Equal(t, want, htt.HasImpliedEndTag(tag), "HasImpliedEndTag(%q)", tag)
	}
}
