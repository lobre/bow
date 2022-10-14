package bow

import (
	"testing"
	"testing/fstest"
)

func TestBuild(t *testing.T) {
	fs := fstest.MapFS{
		"views/index.html": {
			Data: []byte("hello, world"),
		},
	}

	if _, err := NewCore(fs); err != nil {
		t.Fatalf("cannot create core: %v", err)
	}
}
