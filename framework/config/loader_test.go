package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandEnv(t *testing.T) {
	os.Setenv("TEST_ENV_VAR", "my-env-value")
	defer os.Unsetenv("TEST_ENV_VAR")

	input := []byte("url: ${TEST_ENV_VAR}\nfallback: ${NON_EXISTENT_VAR:default-val}\nempty: ${NON_EXISTENT_NO_DEFAULT}")
	expected := "url: my-env-value\nfallback: default-val\nempty: "

	output := expandEnv(input)
	assert.Equal(t, expected, string(output))
}
