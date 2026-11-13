package errs

import (
	"testing"

	"github.com/go-keel/keel/pkg/rule"
)

// Every rule ID used in the repository has a section in docs/admin/startup-errors.md, and every section is still used.
func TestRuleIDsDocumented(t *testing.T) {
	if err := rule.CheckDocs("../../..", Rules); err != nil {
		t.Fatal(err)
	}
}
