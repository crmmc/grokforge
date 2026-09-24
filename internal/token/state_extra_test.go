package token

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPoolToShort_UnknownReturnsInput(t *testing.T) {
	assert.Equal(t, "custom", PoolToShort("custom"))
	assert.Equal(t, "", PoolToShort(""))
}
