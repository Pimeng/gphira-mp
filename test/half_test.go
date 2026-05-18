package test

import (
	"math"
	"testing"

	"github.com/Pimeng/gphira-mp-next/pkg/half"
)

func TestF16BitsToF32(t *testing.T) {
	tests := []struct {
		bits uint16
		want float32
	}{
		{0x0000, 0},
		{0x8000, float32(math.Copysign(0, -1))},
		{0x3c00, 1},
		{0xbc00, -1},
		{0x4000, 2},
		{0x7c00, float32(math.Inf(1))},
		{0xfc00, float32(math.Inf(-1))},
		{0x7e00, float32(math.NaN())},
		{0x7e01, float32(math.NaN())},
		{0x3800, 0.5},
	}
	for _, tt := range tests {
		got := half.F16BitsToF32(tt.bits)
		if math.IsNaN(float64(tt.want)) {
			if !math.IsNaN(float64(got)) {
				t.Errorf("F16BitsToF32(0x%04x) = %v, want NaN", tt.bits, got)
			}
			continue
		}
		if got != tt.want {
			t.Errorf("F16BitsToF32(0x%04x) = %v, want %v", tt.bits, got, tt.want)
		}
	}
}

func TestF32ToF16Bits(t *testing.T) {
	tests := []struct {
		value float32
		want  uint16
	}{
		{0, 0x0000},
		{float32(math.Copysign(0, -1)), 0x8000},
		{1, 0x3c00},
		{-1, 0xbc00},
		{2, 0x4000},
		{float32(math.Inf(1)), 0x7c00},
		{float32(math.Inf(-1)), 0xfc00},
		{float32(math.NaN()), 0x7e00},
		{70000, 0x7c00},
		{-70000, 0xfc00},
	}
	for _, tt := range tests {
		got := half.F32ToF16Bits(tt.value)
		if got != tt.want {
			t.Errorf("F32ToF16Bits(%v) = 0x%04x, want 0x%04x", tt.value, got, tt.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	values := []float32{0, 1, -1, 0.5, -0.5, 2, 100, -100, 0.1, -0.1, 1.5, -2.5}
	for _, v := range values {
		bits := half.F32ToF16Bits(v)
		back := half.F16BitsToF32(bits)
		diff := math.Abs(float64(back-v)) / max(math.Abs(float64(v)), 1)
		if diff >= 0.01 {
			t.Errorf("RoundTrip(%v) = %v (bits=0x%04x), diff=%v", v, back, bits, diff)
		}
	}
}
