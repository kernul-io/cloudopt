package exitcodes_test

import (
	"testing"

	"github.com/kernul-io/cloudopt/internal/domain/exitcodes"
)

func TestExitCodes_areStable(t *testing.T) {
	if exitcodes.Success != 0 {
		t.Errorf("Success = %d", exitcodes.Success)
	}
	if exitcodes.InvalidInput != 2 {
		t.Errorf("InvalidInput = %d", exitcodes.InvalidInput)
	}
	if exitcodes.CollectionFail != 3 {
		t.Errorf("CollectionFail = %d", exitcodes.CollectionFail)
	}
	if exitcodes.AnalysisFail != 4 {
		t.Errorf("AnalysisFail = %d", exitcodes.AnalysisFail)
	}
	if exitcodes.PartialSuccess != 5 {
		t.Errorf("PartialSuccess = %d", exitcodes.PartialSuccess)
	}
}
