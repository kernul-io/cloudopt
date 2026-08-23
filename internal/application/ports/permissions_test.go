package ports_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/ports"
)

func TestErrMissingPermissions(t *testing.T) {
	t.Parallel()

	require.Nil(t, ports.ErrMissingPermissions(nil))
	require.Nil(t, ports.ErrMissingPermissions([]string{}))

	err := ports.ErrMissingPermissions([]string{"cloudwatch:GetMetricData", "cloudwatch:ListMetrics"})
	require.Error(t, err)
	require.Equal(t, "missing IAM permissions: cloudwatch:GetMetricData, cloudwatch:ListMetrics", err.Error())
	require.True(t, ports.IsMissingPermissions(err))
	require.False(t, ports.IsMissingPermissions(errors.New("other")))
}

func TestErrMissingPermissions_wraps(t *testing.T) {
	t.Parallel()

	err := ports.ErrMissingPermissions([]string{"ec2:DescribeInstances"})
	wrapped := errors.Join(errors.New("collect failed"), err)
	require.True(t, ports.IsMissingPermissions(wrapped))
}
