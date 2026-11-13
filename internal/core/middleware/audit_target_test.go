package middleware

import (
	"strings"
	"testing"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
)

// compositeReq has both an id and a self-declared target: requests with a composite key often have an id field,
// but it is only one of three parts, and recording it alone records the wrong thing.
type compositeReq struct{ target string }

func (compositeReq) GetId() int64          { return 3 }
func (r compositeReq) AuditTarget() string { return r.target }

// TestAuditTargetID checks that the self-declared target wins over the id field; the wrong order raises no error, it
// only records the wrong object quietly.
func TestAuditTargetID(t *testing.T) {
	cases := []struct {
		name string
		req  any
		want string
	}{
		{"self-declared wins", compositeReq{"COLLECTION/WAKEUP/3"}, "COLLECTION/WAKEUP/3"},
		{"falls back to id", &v1.ResetPasswordRequest{Id: 7}, "7"},
		{"zero id stays empty", &v1.ResetPasswordRequest{}, ""},
		{"too long is cut to the column width", compositeReq{strings.Repeat("x", 70)}, strings.Repeat("x", targetIDMaxLen)},
	}
	for _, c := range cases {
		if got := auditTargetID(c.req); got != c.want {
			t.Errorf("%s: auditTargetID = %q, want %q", c.name, got, c.want)
		}
	}
}
