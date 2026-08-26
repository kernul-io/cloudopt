package security_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLeastPrivilegeIAMFixtures_parse(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	adapters := filepath.Join(root, "internal", "adapters")
	var files []string
	require.NoError(t, filepath.Walk(adapters, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), "iam-least-privilege.json") {
			files = append(files, path)
		}
		return nil
	}))
	require.NotEmpty(t, files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err, f)
		var raw json.RawMessage
		require.NoError(t, json.Unmarshal(data, &raw), f)
	}
}
