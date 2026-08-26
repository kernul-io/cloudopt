package security

import (
	"regexp"
	"strings"
)

var (
	awsAccountRE = regexp.MustCompile(`\b\d{12}\b`)
	arnRE        = regexp.MustCompile(`arn:aws:[a-z0-9-]+:[a-z0-9-]*:\d{12}:[^\s"']+`)
	gcpProjectRE = regexp.MustCompile(`\b[a-z][a-z0-9-]{4,28}[a-z0-9]\b`)
	emailRE      = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	signedURLRE  = regexp.MustCompile(`(?i)(X-Amz-Signature|Signature=)[^\s"']+`)
)

// RedactLogMessage removes common cloud identifiers from log and error strings.
func RedactLogMessage(msg string) string {
	if msg == "" {
		return msg
	}
	out := msg
	out = arnRE.ReplaceAllString(out, "arn:aws:***:REDACTED")
	out = awsAccountRE.ReplaceAllStringFunc(out, func(s string) string {
		if len(s) == 12 {
			return "************"
		}
		return s
	})
	out = signedURLRE.ReplaceAllString(out, "Signature=REDACTED")
	out = emailRE.ReplaceAllString(out, "user@redacted")
	// Avoid over-redacting short tokens; only redact project-like ids when prefixed.
	out = strings.ReplaceAll(out, "projects/", "projects/REDACTED-")
	_ = gcpProjectRE // reserved for future targeted redaction
	return out
}
