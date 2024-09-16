package main

import (
	"slices"
	"testing"
)

func TestFiller(t *testing.T) {
	b := [100]byte{}
	zero := byte('0')
	one := byte('1')
	filler(b[:], zero, one)
	if !slices.Contains(b[:], zero) {
		t.Errorf("Массив не содержит символ '0'")
	}
	if !slices.Contains(b[:], one) {
		t.Errorf("Массив не содержит символ '1")
	}
}
