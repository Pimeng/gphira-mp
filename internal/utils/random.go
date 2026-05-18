package utils

import (
	"math/rand"
)

// RandomPickInt32 returns a random element from the slice.
// Returns 0 if the slice is empty.
func RandomPickInt32(ids []int32) int32 {
	if len(ids) == 0 {
		return 0
	}
	return ids[rand.Intn(len(ids))]
}
