package view_test

import (
	"testing"

	"github.com/derailed/k9s/internal/resource"
	"github.com/derailed/k9s/internal/view"
	"github.com/stretchr/testify/assert"
)

func TestSecretNew(t *testing.T) {
	s := view.NewSecret("secrets", "", resource.NewSecretList(nil, ""))
	s.Init(makeCtx())

	assert.Equal(t, "secrets", s.Name())
	assert.Equal(t, 19, len(s.Hints()))
}