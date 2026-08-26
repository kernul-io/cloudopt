package security_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/security"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestValidateOutputPath(t *testing.T) {
	root := t.TempDir()
	absRoot, err := filepath.Abs(root)
	require.NoError(t, err)
	good := filepath.Join(absRoot, "out.json")
	require.NoError(t, security.ValidateOutputPath(good, absRoot))
	require.Error(t, security.ValidateOutputPath("relative/out.json", absRoot))
	escape := filepath.Join(absRoot, "..", "escape.json")
	require.Error(t, security.ValidateOutputPath(escape, absRoot))
}

func TestRedactLogMessage(t *testing.T) {
	msg := "caller arn:aws:iam::123456789012:user/admin failed for account 123456789012"
	out := security.RedactLogMessage(msg)
	require.NotContains(t, out, "123456789012")
	require.Contains(t, out, "REDACTED")
}

func TestEncryptMetadata_roundTrip(t *testing.T) {
	t.Setenv("COA_TEST_KEY", "test-secret-material")
	plain := []byte(`{"customer":"acme"}`)
	enc, err := security.EncryptMetadata("COA_TEST_KEY", plain)
	require.NoError(t, err)
	got, err := security.DecryptMetadata("COA_TEST_KEY", enc)
	require.NoError(t, err)
	require.Equal(t, plain, got)
}

func TestEncryptMetadata_plainWhenUnset(t *testing.T) {
	enc, err := security.EncryptMetadata("", []byte("x"))
	require.NoError(t, err)
	got, err := security.DecryptMetadata("", enc)
	require.NoError(t, err)
	require.Equal(t, []byte("x"), got)
}

func TestCheckDatabaseCompatibility_failClosed(t *testing.T) {
	require.NoError(t, security.CheckDatabaseCompatibility(1, 6))
	require.Error(t, security.CheckDatabaseCompatibility(99, 6))
	require.NoError(t, security.CheckDatabaseCompatibility(0, 6))
}

func TestSecureFile_permissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.json")
	require.NoError(t, security.SecureFile(path, []byte("{}")))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestCollectionBudget_validate(t *testing.T) {
	b := security.DefaultBudget(types.ProviderAWS)
	require.Equal(t, 8, b.ClampConcurrency(99))
	require.Equal(t, 4, b.ClampConcurrency(4))
}
