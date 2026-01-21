package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestPathType_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		want    PathType
		wantErr bool
	}{
		{name: "File", input: "file", want: File, wantErr: false},
		{name: "Directory", input: "directory", want: Directory, wantErr: false},
		{name: "Invalid", input: "socket", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var pt PathType
			err := yaml.Unmarshal([]byte(tc.input), &pt)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, pt)
		})
	}
}

func TestPathType_String(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "file", File.String())
	assert.Equal(t, "directory", Directory.String())
	var unknown PathType = 99
	assert.Equal(t, "unknown", unknown.String())
}
