package types

import (
	"fmt"
	"time"
)

// Bytes is a typed storage size.
type Bytes int64

func (b Bytes) String() string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Percentage stores values in basis points (10000 = 100.00%).
type Percentage struct {
	BasisPoints int64
}

func (p Percentage) Float64() float64 {
	return float64(p.BasisPoints) / 10000.0
}

func PercentageFromFloat(v float64) Percentage {
	return Percentage{BasisPoints: int64(v * 10000)}
}

// Duration wraps time.Duration for domain clarity.
type Duration struct {
	Value time.Duration
}

// Timestamp is an RFC3339-nano instant in UTC for canonical storage.
type Timestamp struct {
	time.Time
}

func NewTimestamp(t time.Time) Timestamp {
	return Timestamp{Time: t.UTC()}
}

// NowUTC returns the current instant as a canonical timestamp.
func NowUTC() Timestamp {
	return NewTimestamp(time.Now())
}

func ParseTimestamp(s string) (Timestamp, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return Timestamp{}, err
	}
	return NewTimestamp(t), nil
}

func (t Timestamp) Canonical() string {
	return t.UTC().Format(time.RFC3339Nano)
}
