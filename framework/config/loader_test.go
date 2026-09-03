package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST_ENV_VAR", "my-env-value")

	input := []byte("url: ${TEST_ENV_VAR}\nfallback: ${NON_EXISTENT_VAR:default-val}\nempty: ${NON_EXISTENT_NO_DEFAULT}")
	expected := "url: my-env-value\nfallback: default-val\nempty: "

	output := expandEnv(input)
	assert.Equal(t, expected, string(output))
}
