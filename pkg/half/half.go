// Package half provides conversions between IEEE 754 half-precision (float16)
// and single-precision (float32) floating-point numbers.
package half

import "math"

// F16BitsToF32 converts a float16 bit pattern to float32.
func F16BitsToF32(bits uint16) float32 {
	sign := float32(1)
	if bits&0x8000 != 0 {
		sign = -1
	}
	exp := (bits >> 10) & 0x1f
	frac := bits & 0x03ff

	if exp == 0 {
		if frac == 0 {
			return sign * 0
		}
		return sign * float32(math.Pow(2, -14)) * (float32(frac) / 1024)
	}

	if exp == 0x1f {
		if frac == 0 {
			return sign * float32(math.Inf(1))
		}
		return float32(math.NaN())
	}

	return sign * float32(math.Pow(2, float64(exp)-15)) * (1 + float32(frac)/1024)
}

// F32ToF16Bits converts a float32 value to float16 bit pattern.
func F32ToF16Bits(value float32) uint16 {
	v := float64(value)
	if math.IsNaN(v) {
		return 0x7e00
	}
	if math.IsInf(v, 1) {
		return 0x7c00
	}
	if math.IsInf(v, -1) {
		return 0xfc00
	}

	sign := uint16(0)
	if v < 0 || (v == 0 && math.Signbit(v)) {
		sign = 0x8000
	}
	abs := math.Abs(v)

	if abs == 0 {
		return sign
	}

	exp := math.Floor(math.Log2(abs))
	frac := abs/math.Pow(2, exp) - 1

	halfExp := int(exp) + 15
	if halfExp >= 0x1f {
		return sign | 0x7c00
	}

	if halfExp <= 0 {
		sub := int(math.Round(abs / math.Pow(2, -14) * 1024))
		if sub <= 0 {
			return sign
		}
		return sign | (uint16(sub) & 0x03ff)
	}

	halfFrac := int(math.Round(frac * 1024))
	if halfFrac == 1024 {
		nextExp := halfExp + 1
		if nextExp >= 0x1f {
			return sign | 0x7c00
		}
		return sign | (uint16(nextExp) << 10)
	}

	return sign | (uint16(halfExp) << 10) | (uint16(halfFrac) & 0x03ff)
}
